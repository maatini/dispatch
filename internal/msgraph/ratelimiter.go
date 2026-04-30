package msgraph

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter manages per-sender token-bucket rate limiters.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	skipWait bool
}

func NewRateLimiter(skipWait bool) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		skipWait: skipWait,
	}
}

func (rl *RateLimiter) Wait(ctx context.Context, senderEmail string) error {
	if rl.skipWait {
		return nil
	}
	lim := rl.get(senderEmail)
	return lim.Wait(ctx)
}

func (rl *RateLimiter) get(senderEmail string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	lim, ok := rl.limiters[senderEmail]
	if !ok {
		lim = rate.NewLimiter(1, 10) // 1 req/s, burst 10
		rl.limiters[senderEmail] = lim
	}
	return lim
}
