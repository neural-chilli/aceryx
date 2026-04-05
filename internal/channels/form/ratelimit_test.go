package form

import (
	"testing"
	"time"
)

func TestIPRateLimiterWindowAndLimit(t *testing.T) {
	t.Parallel()

	rl := NewIPRateLimiter(time.Minute)
	now := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		if !rl.Allow("1.1.1.1", 10, now) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("1.1.1.1", 10, now) {
		t.Fatalf("11th request should be blocked")
	}
	if !rl.Allow("1.1.1.1", 10, now.Add(time.Minute+time.Second)) {
		t.Fatalf("request after window expiry should be allowed")
	}
}
