package sender

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

// DefaultCacheTTL is the default in-memory cache duration for sender lookups.
const DefaultCacheTTL = 10 * time.Minute

// KV is the minimal KV store interface required by Store.
type KV interface {
	Get(key string) (nats.KeyValueEntry, error)
	Put(key string, value []byte) (uint64, error)
	Create(key string, value []byte) (uint64, error)
	Delete(key string, opts ...nats.DeleteOpt) error
	Keys(opts ...nats.WatchOpt) ([]string, error)
}

type cacheEntry struct {
	sender    domain.Sender
	expiresAt time.Time
}

// Store wraps the NATS KV bucket for sender configuration with an in-memory TTL cache.
type Store struct {
	kv       KV
	cacheTTL time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

func New(kv KV, cacheTTL time.Duration) *Store {
	return &Store{
		kv:       kv,
		cacheTTL: cacheTTL,
		cache:    make(map[string]cacheEntry),
	}
}

func (s *Store) Get(appTag string) (domain.Sender, error) {
	s.mu.RLock()
	entry, ok := s.cache[appTag]
	s.mu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.sender, nil
	}

	kve, err := s.kv.Get(appTag)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return domain.Sender{}, &domain.ValidationError{
			Code:    domain.ErrUnknownAppTag,
			Message: fmt.Sprintf("unknown appTag: %s", appTag),
		}
	}
	if err != nil {
		return domain.Sender{}, fmt.Errorf("sender KV get %s: %w", appTag, err)
	}

	var sender domain.Sender
	if err := json.Unmarshal(kve.Value(), &sender); err != nil {
		return domain.Sender{}, fmt.Errorf("sender unmarshal %s: %w", appTag, err)
	}

	s.mu.Lock()
	s.cache[appTag] = cacheEntry{sender: sender, expiresAt: time.Now().Add(s.cacheTTL)}
	s.mu.Unlock()

	return sender, nil
}

func (s *Store) Put(sender domain.Sender) error {
	data, err := json.Marshal(sender)
	if err != nil {
		return fmt.Errorf("sender marshal %s: %w", sender.AppTag, err)
	}
	if _, err := s.kv.Put(sender.AppTag, data); err != nil {
		return fmt.Errorf("sender KV put %s: %w", sender.AppTag, err)
	}
	s.mu.Lock()
	delete(s.cache, sender.AppTag)
	s.mu.Unlock()
	return nil
}

func (s *Store) Delete(appTag string) error {
	if err := s.kv.Delete(appTag); err != nil {
		return fmt.Errorf("sender KV delete %s: %w", appTag, err)
	}
	s.mu.Lock()
	delete(s.cache, appTag)
	s.mu.Unlock()
	return nil
}

func (s *Store) List() ([]domain.Sender, error) {
	keys, err := s.kv.Keys()
	if err != nil {
		if errors.Is(err, nats.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("sender KV keys: %w", err)
	}
	senders := make([]domain.Sender, 0, len(keys))
	for _, key := range keys {
		sender, err := s.Get(key)
		if err != nil {
			return nil, err
		}
		senders = append(senders, sender)
	}
	return senders, nil
}
