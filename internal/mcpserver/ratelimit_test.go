package mcpserver

import (
	"testing"

	"github.com/google/uuid"
)

func TestRateLimiterPerTool(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 1000, ToolLimits: map[string]int{"create_case": 1}})
	keyID := uuid.New()
	if err := rl.Allow(keyID, "create_case"); err != nil {
		t.Fatalf("first allow failed: %v", err)
	}
	if err := rl.Allow(keyID, "create_case"); err == nil {
		t.Fatalf("expected per-tool rate limit error")
	}
}
