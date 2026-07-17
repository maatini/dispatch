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

type kvStore interface {
	Get(key string) (nats.KeyValueEntry, error)
	Put(key string, value []byte) (uint64, error)
	Create(key string, value []byte) (uint64, error)
	Delete(key string, opts ...nats.DeleteOpt) error
	Keys(opts ...nats.WatchOpt) ([]string, error)
}

// KVStore is the exported test interface for the KV store.
type KVStore = kvStore

type cacheEntry struct {
	sender    domain.Sender
	expiresAt time.Time
}

// CacheEntry is exported for test access.
type CacheEntry = cacheEntry

// Store wraps the NATS KV bucket for sender configuration with an in-memory TTL cache.
type Store struct {
	Kv       kvStore
	CacheTTL time.Duration

	Mu    sync.RWMutex
	Cache map[string]cacheEntry
}

func New(kv nats.KeyValue, cacheTTL time.Duration) *Store {
	return &Store{
		Kv:       kv,
		CacheTTL: cacheTTL,
		Cache:    make(map[string]cacheEntry),
	}
}

func (s *Store) Get(appTag string) (domain.Sender, error) {
	s.Mu.RLock()
	entry, ok := s.Cache[appTag]
	s.Mu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.sender, nil
	}

	kve, err := s.Kv.Get(appTag)
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

	s.Mu.Lock()
	s.Cache[appTag] = CacheEntry{sender: sender, expiresAt: time.Now().Add(s.CacheTTL)}
	s.Mu.Unlock()

	return sender, nil
}

func (s *Store) Put(sender domain.Sender) error {
	data, err := json.Marshal(sender)
	if err != nil {
		return fmt.Errorf("sender marshal %s: %w", sender.AppTag, err)
	}
	if _, err := s.Kv.Put(sender.AppTag, data); err != nil {
		return fmt.Errorf("sender KV put %s: %w", sender.AppTag, err)
	}
	s.Mu.Lock()
	delete(s.Cache, sender.AppTag)
	s.Mu.Unlock()
	return nil
}

func (s *Store) Delete(appTag string) error {
	if err := s.Kv.Delete(appTag); err != nil {
		return fmt.Errorf("sender KV delete %s: %w", appTag, err)
	}
	s.Mu.Lock()
	delete(s.Cache, appTag)
	s.Mu.Unlock()
	return nil
}

func (s *Store) List() ([]domain.Sender, error) {
	keys, err := s.Kv.Keys()
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

func (s *Store) InvalidateCache(appTag string) {
	s.Mu.Lock()
	delete(s.Cache, appTag)
	s.Mu.Unlock()
}
