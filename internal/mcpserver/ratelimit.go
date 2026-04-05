package mcpserver

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

type toolLimitKey struct {
	KeyID    uuid.UUID
	ToolName string
}

type RateLimiter struct {
	mu           sync.RWMutex
	keyLimiters  map[uuid.UUID]*rate.Limiter
	toolLimiters map[toolLimitKey]*rate.Limiter
	config       RateLimitConfig
}

func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	if config.RequestsPerMinute <= 0 {
		config.RequestsPerMinute = DefaultRequestsPerMinute
	}
	if config.ToolLimits == nil {
		config.ToolLimits = map[string]int{}
	}
	return &RateLimiter{
		keyLimiters:  map[uuid.UUID]*rate.Limiter{},
		toolLimiters: map[toolLimitKey]*rate.Limiter{},
		config:       config,
	}
}

func limiterForPerMinute(perMinute int) *rate.Limiter {
	if perMinute <= 0 {
		perMinute = 1
	}
	perSecond := float64(perMinute) / float64(time.Minute/time.Second)
	return rate.NewLimiter(rate.Limit(perSecond), perMinute)
}

func (rl *RateLimiter) Allow(keyID uuid.UUID, toolName string) error {
	if rl == nil {
		return nil
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, ok := rl.keyLimiters[keyID]
	if !ok {
		limiter = limiterForPerMinute(rl.config.RequestsPerMinute)
		rl.keyLimiters[keyID] = limiter
	}
	if !limiter.Allow() {
		return fmt.Errorf("rate limit exceeded for API key")
	}

	perToolLimit := rl.config.ToolLimits[toolName]
	if perToolLimit <= 0 {
		return nil
	}
	k := toolLimitKey{KeyID: keyID, ToolName: toolName}
	toolLimiter, ok := rl.toolLimiters[k]
	if !ok {
		toolLimiter = limiterForPerMinute(perToolLimit)
		rl.toolLimiters[k] = toolLimiter
	}
	if !toolLimiter.Allow() {
		return fmt.Errorf("rate limit exceeded for tool %s", toolName)
	}
	return nil
}
