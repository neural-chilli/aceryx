package mcp

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestDepthChecks(t *testing.T) {
	if err := CheckDepth(2, 3); err != nil {
		t.Fatalf("expected allowed depth: %v", err)
	}
	if err := CheckDepth(4, 3); err == nil {
		t.Fatalf("expected depth error")
	}
}

func TestDepthHeaderSet(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	setDepthHeader(req, 2)
	if req.Header.Get("X-MCP-Depth") != "2" {
		t.Fatalf("expected depth header")
	}
}

func TestGetDepthFromContext(t *testing.T) {
	ctx := WithDepth(context.Background(), 3)
	if got := getDepthFromContext(ctx); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}
