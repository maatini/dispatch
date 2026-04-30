package quota

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

type kvStore interface {
	Get(key string) (nats.KeyValueEntry, error)
	Create(key string, value []byte) (uint64, error)
	Update(key string, value []byte, last uint64) (uint64, error)
}

type entry struct {
	TS    int64 `json:"ts"`
	Count int   `json:"count"`
}

type state struct {
	Entries []entry `json:"entries"`
}

const maxCASRetries = 10

// Checker implements rolling 24h quota via NATS KV with optimistic CAS.
type Checker struct {
	kv kvStore
}

func NewChecker(kv nats.KeyValue) *Checker {
	return &Checker{kv: kv}
}

// Check verifies and records recipient usage. Returns QuotaError if exceeded,
// QuotaStateError if the KV store is unavailable (fail-closed).
func (c *Checker) Check(appTag string, limit, requested int) error {
	if limit <= 0 {
		return nil // unlimited
	}
	for i := range maxCASRetries {
		err := c.attempt(appTag, limit, requested)
		if err == nil {
			return nil
		}
		var casErr *casConflict
		if errors.As(err, &casErr) {
			if i == maxCASRetries-1 {
				return &domain.QuotaStateError{Cause: fmt.Errorf("CAS conflict after %d retries for %s", maxCASRetries, appTag)}
			}
			continue
		}
		return err
	}
	return nil
}

type casConflict struct{}

func (e *casConflict) Error() string { return "CAS conflict" }

func (c *Checker) attempt(appTag string, limit, requested int) error {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour).Unix()

	kve, err := c.kv.Get(appTag)
	var revision uint64
	var current state

	if errors.Is(err, nats.ErrKeyNotFound) {
		revision = 0
	} else if err != nil {
		return &domain.QuotaStateError{Cause: fmt.Errorf("quota KV get %s: %w", appTag, err)}
	} else {
		revision = kve.Revision()
		if jsonErr := json.Unmarshal(kve.Value(), &current); jsonErr != nil {
			return &domain.QuotaStateError{Cause: fmt.Errorf("quota unmarshal %s: %w", appTag, jsonErr)}
		}
	}

	// filter expired entries
	filtered := current.Entries[:0]
	sum := 0
	for _, e := range current.Entries {
		if e.TS > cutoff {
			filtered = append(filtered, e)
			sum += e.Count
		}
	}

	if sum+requested > limit {
		return &domain.QuotaError{Limit: limit, Current: sum, Requested: requested}
	}

	next := state{Entries: append(filtered, entry{TS: now.Unix(), Count: requested})}
	data, err := json.Marshal(next)
	if err != nil {
		return &domain.QuotaStateError{Cause: fmt.Errorf("quota marshal %s: %w", appTag, err)}
	}

	if revision == 0 {
		_, err = c.kv.Create(appTag, data)
	} else {
		_, err = c.kv.Update(appTag, data, revision)
	}

	if err != nil {
		// JetStream API errors (wrong sequence, key exists) → CAS conflict → retry.
		// Network/connection errors are not JetStreamErrors → fail-closed.
		var jsErr nats.JetStreamError
		if errors.As(err, &jsErr) {
			return &casConflict{}
		}
		return &domain.QuotaStateError{Cause: fmt.Errorf("quota KV write %s: %w", appTag, err)}
	}
	return nil
}

// CurrentUsage returns the rolling 24h recipient count for an appTag (for headers/metrics).
func (c *Checker) CurrentUsage(appTag string) (int, error) {
	kve, err := c.kv.Get(appTag)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("quota KV get %s: %w", appTag, err)
	}
	var current state
	if err := json.Unmarshal(kve.Value(), &current); err != nil {
		return 0, fmt.Errorf("quota unmarshal %s: %w", appTag, err)
	}
	cutoff := time.Now().Add(-24 * time.Hour).Unix()
	sum := 0
	for _, e := range current.Entries {
		if e.TS > cutoff {
			sum += e.Count
		}
	}
	return sum, nil
}
