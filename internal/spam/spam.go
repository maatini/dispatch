package spam

import (
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"

	"codymail-go/internal/domain"
)

type kvStore interface {
	Get(key string) (nats.KeyValueEntry, error)
	Put(key string, value []byte) (uint64, error)
}

// Checker detects duplicate mail submissions using a NATS KV bucket with TTL.
type Checker struct {
	kv kvStore
}

func NewChecker(kv nats.KeyValue) *Checker {
	return &Checker{kv: kv}
}

// Check returns a ValidationError if the hash was seen within the bucket TTL,
// otherwise records the hash and returns nil.
func (c *Checker) Check(hash string) error {
	_, err := c.kv.Get(hash)
	if err == nil {
		return &domain.ValidationError{
			Code:    domain.ErrSpamDetected,
			Message: "duplicate message detected within spam window",
		}
	}
	if !errors.Is(err, nats.ErrKeyNotFound) {
		return fmt.Errorf("spam KV get %s: %w", hash, err)
	}

	if _, err := c.kv.Put(hash, []byte{1}); err != nil {
		return fmt.Errorf("spam KV put %s: %w", hash, err)
	}
	return nil
}
