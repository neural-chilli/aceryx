package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientDiscover(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"a","description":"A","inputSchema":{"type":"object"}},{"name":"b","description":"B","inputSchema":{"type":"object"}},{"name":"c","description":"C","inputSchema":{"type":"object"}}]}}`))
	}))
	defer srv.Close()

	tools, err := NewClient(srv.URL, &http.Client{Timeout: time.Second}, AuthConfig{Type: "none"}).Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
	if tools[0].Name != "a" || tools[1].Name != "b" || tools[2].Name != "c" {
		t.Fatalf("unexpected tool names: %+v", tools)
	}
}

func TestClientInvokeAndErrorFlag(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"done"}],"isError":false}}`))
		}))
		defer srv.Close()
		result, err := NewClient(srv.URL, nil, AuthConfig{Type: "none"}).Invoke(context.Background(), "x", json.RawMessage(`{"a":1}`))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected isError=false")
		}
	})

	t.Run("tool-error-not-transport-error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"bad"}],"isError":true}}`))
		}))
		defer srv.Close()
		result, err := NewClient(srv.URL, nil, AuthConfig{Type: "none"}).Invoke(context.Background(), "x", json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if !result.IsError {
			t.Fatalf("expected isError=true")
		}
	})
}

func TestClientErrors(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		_, err := NewClient("http://127.0.0.1:1", &http.Client{Timeout: 50 * time.Millisecond}, AuthConfig{Type: "none"}).Discover(context.Background())
		if err == nil || !strings.Contains(err.Error(), "MCP server unreachable") {
			t.Fatalf("expected unreachable error, got %v", err)
		}
	})

	t.Run("invalid-json-rpc", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{`))
		}))
		defer srv.Close()
		_, err := NewClient(srv.URL, nil, AuthConfig{Type: "none"}).Discover(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid JSON-RPC response") {
			t.Fatalf("expected invalid response error, got %v", err)
		}
	})
}

func TestClientAuthHeaders(t *testing.T) {
	t.Run("bearer", func(t *testing.T) {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
		}))
		defer srv.Close()
		_, _ = NewClient(srv.URL, nil, AuthConfig{Type: "bearer", SecretRef: "tok"}).Discover(context.Background())
		if got != "Bearer tok" {
			t.Fatalf("expected bearer header, got %q", got)
		}
	})

	t.Run("none", func(t *testing.T) {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
		}))
		defer srv.Close()
		_, _ = NewClient(srv.URL, nil, AuthConfig{Type: "none"}).Discover(context.Background())
		if got != "" {
			t.Fatalf("expected no auth header, got %q", got)
		}
	})
}
