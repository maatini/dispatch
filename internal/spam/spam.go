package spam

import (
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

type kvStore interface {
	Create(key string, value []byte) (uint64, error)
}

// Checker detects duplicate mail submissions using a NATS KV bucket with TTL.
type Checker struct {
	kv kvStore
}

func NewChecker(kv nats.KeyValue) *Checker {
	return &Checker{kv: kv}
}

// Check returns a ValidationError if the hash was seen within the bucket TTL,
// otherwise records the hash and returns nil. Recording is atomic via KV Create.
func (c *Checker) Check(hash string) error {
	if _, err := c.kv.Create(hash, []byte{1}); err != nil {
		if errors.Is(err, nats.ErrKeyExists) {
			return &domain.ValidationError{
				Code:    domain.ErrSpamDetected,
				Message: "duplicate message detected within spam window",
			}
		}
		return &domain.SpamStateError{Cause: fmt.Errorf("spam KV create %s: %w", hash, err)}
	}
	return nil
}
