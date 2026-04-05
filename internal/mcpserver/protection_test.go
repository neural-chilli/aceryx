package mcpserver

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReadDepthAndCheckDepth(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-MCP-Depth", "2")
	if got := ReadDepth(req); got != 2 {
		t.Fatalf("expected depth=2, got %d", got)
	}
	if err := CheckDepth(2, 3); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := CheckDepth(4, 3); err == nil {
		t.Fatalf("expected depth error")
	}
}

func TestApplyTimeout(t *testing.T) {
	ctx := context.Background()
	nctx, cancel := ApplyTimeout(ctx, 5*time.Second, 120*time.Second)
	defer cancel()
	deadline, ok := nctx.Deadline()
	if !ok {
		t.Fatalf("expected deadline")
	}
	if left := time.Until(deadline); left > 6*time.Second || left < 4*time.Second {
		t.Fatalf("expected ~5s deadline, got %s", left)
	}
}
