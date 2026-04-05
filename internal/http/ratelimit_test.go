package httpfw

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimitPluginLimitDelays(t *testing.T) {
	rlm := NewRateLimitManager(RateLimitDefaults{GlobalPerTenant: rate.Limit(1000), GlobalBurst: 1000})
	rlm.SetPluginLimit("t1", "p1", 2, 1)

	if err := rlm.Wait(context.Background(), "t1", "p1", "api.example.com"); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	start := time.Now()
	if err := rlm.Wait(context.Background(), "t1", "p1", "api.example.com"); err != nil {
		t.Fatalf("second wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Fatalf("expected throttling delay, got %v", elapsed)
	}
}

func TestRateLimitDomainPause(t *testing.T) {
	rlm := NewRateLimitManager(RateLimitDefaults{GlobalPerTenant: rate.Limit(1000), GlobalBurst: 1000})
	rlm.PauseDomain("t1", "api.example.com", time.Now().Add(120*time.Millisecond))

	start := time.Now()
	if err := rlm.Wait(context.Background(), "t1", "p1", "api.example.com"); err != nil {
		t.Fatalf("wait with pause: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("expected pause delay, got %v", elapsed)
	}
}

func TestRateLimitGlobalTenantLimitAcrossPlugins(t *testing.T) {
	rlm := NewRateLimitManager(RateLimitDefaults{GlobalPerTenant: rate.Limit(1), GlobalBurst: 1})

	if err := rlm.Wait(context.Background(), "t1", "p1", "api.example.com"); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	start := time.Now()
	if err := rlm.Wait(context.Background(), "t1", "p2", "api.example.com"); err != nil {
		t.Fatalf("second wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("expected global tenant throttling delay, got %v", elapsed)
	}
}

func TestRateLimitWaitContextCancelled(t *testing.T) {
	rlm := NewRateLimitManager(RateLimitDefaults{GlobalPerTenant: rate.Limit(1000), GlobalBurst: 1000})
	rlm.PauseDomain("t1", "api.example.com", time.Now().Add(2*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := rlm.Wait(ctx, "t1", "p1", "api.example.com"); err == nil {
		t.Fatal("expected context cancellation error")
	}
}
