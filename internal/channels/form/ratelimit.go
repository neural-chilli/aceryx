package form

import (
	"sync"
	"time"
)

type rateWindow struct {
	count     int
	expiresAt time.Time
}

type IPRateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	state  map[string]rateWindow
}

func NewIPRateLimiter(window time.Duration) *IPRateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &IPRateLimiter{
		window: window,
		state:  map[string]rateWindow{},
	}
}

func (rl *IPRateLimiter) Allow(key string, limit int, now time.Time) bool {
	if rl == nil {
		return true
	}
	if limit <= 0 {
		limit = 10
	}
	if key == "" {
		key = "unknown"
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.state == nil {
		rl.state = map[string]rateWindow{}
	}

	for k, slot := range rl.state {
		if !slot.expiresAt.After(now) {
			delete(rl.state, k)
		}
	}

	slot := rl.state[key]
	if !slot.expiresAt.After(now) {
		slot = rateWindow{count: 0, expiresAt: now.Add(rl.window)}
	}
	if slot.count >= limit {
		rl.state[key] = slot
		return false
	}
	slot.count++
	rl.state[key] = slot
	return true
}
