package mcp

import (
	"testing"
	"time"
)

func TestCircuitBreakerLifecycle(t *testing.T) {
	cb := NewCircuitBreaker(20*time.Millisecond, 2, time.Minute)
	if err := cb.Allow(); err != nil {
		t.Fatalf("allow in closed should pass: %v", err)
	}
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}
	if err := cb.Allow(); err == nil {
		t.Fatalf("expected immediate reject while open")
	}
	time.Sleep(25 * time.Millisecond)
	if err := cb.Allow(); err != nil {
		t.Fatalf("expected half-open probe allowed: %v", err)
	}
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after success probe")
	}

	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(25 * time.Millisecond)
	if err := cb.Allow(); err != nil {
		t.Fatalf("expected probe allowed: %v", err)
	}
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected reopen after failed probe")
	}
}
