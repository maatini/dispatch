package msgraph

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_Wait_SkipWait(t *testing.T) {
	rl := NewRateLimiter(true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := rl.Wait(ctx, "sender@example.com"); err != nil {
		t.Fatalf("skipWait must return nil immediately, got: %v", err)
	}
}

func TestRateLimiter_Get_PerSenderIsolation(t *testing.T) {
	rl := NewRateLimiter(false)
	limA1 := rl.get("a@example.com")
	limA2 := rl.get("a@example.com")
	limB := rl.get("b@example.com")

	if limA1 != limA2 {
		t.Error("same sender must return same limiter")
	}
	if limA1 == limB {
		t.Error("different senders must return different limiters")
	}
}

func TestRateLimiter_Wait_BurstExhausted(t *testing.T) {
	rl := NewRateLimiter(false)

	// Burst = 10, so the first 10 calls should pass immediately.
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := rl.Wait(ctx, "sender@example.com")
		cancel()
		if err != nil {
			t.Fatalf("call %d within burst must not block, got: %v", i, err)
		}
	}

	// 11th call should block because the bucket is empty and the rate is 1/s.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Millisecond))
	defer cancel()
	err := rl.Wait(ctx, "sender@example.com")
	if err == nil {
		t.Fatal("11th call after burst exhaustion must block and fail with context deadline")
	}
}
