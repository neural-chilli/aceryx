package mcp

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCascadeTimeout(t *testing.T) {
	ctxA, cancelA := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancelA()
	c1, cancel1 := CascadeTimeout(ctxA, 30000)
	defer cancel1()
	d1, _ := c1.Deadline()
	if rem := time.Until(d1); rem > 31*time.Second || rem < 28*time.Second {
		t.Fatalf("expected ~30s timeout, got %v", rem)
	}

	ctxB, cancelB := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelB()
	c2, cancel2 := CascadeTimeout(ctxB, 30000)
	defer cancel2()
	d2, _ := c2.Deadline()
	if rem := time.Until(d2); rem > 11*time.Second || rem < 8*time.Second {
		t.Fatalf("expected ~10s timeout, got %v", rem)
	}
}

func TestTimeoutHeaderSet(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	setTimeoutHeader(req, 1500*time.Millisecond)
	if req.Header.Get("X-MCP-Timeout-Ms") != "1500" {
		t.Fatalf("expected timeout header")
	}
}
