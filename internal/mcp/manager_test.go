package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeSecrets struct{ values map[string]string }

func (f fakeSecrets) Get(_ context.Context, _ uuid.UUID, key string) (string, error) {
	return f.values[key], nil
}

func TestManagerInvokeAndCircuit(t *testing.T) {
	tenantID := uuid.New()
	t.Run("invoke-success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"ok"}],"isError":false}}`))
		}))
		defer srv.Close()
		m := NewManager(nil, fakeSecrets{values: map[string]string{"k": "secret"}}, nil, nil)
		result, err := m.InvokeTool(context.Background(), InvokeRequest{TenantID: tenantID, ServerURL: srv.URL, Auth: AuthConfig{Type: "bearer", SecretRef: "k"}, ToolName: "x", Arguments: []byte(`{}`), TimeoutMS: 1000})
		if err != nil {
			t.Fatalf("InvokeTool: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected non-error tool result")
		}
	})

	t.Run("circuit-opens", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		m := NewManager(nil, fakeSecrets{values: map[string]string{"k": "secret"}}, nil, nil)
		for i := 0; i < 5; i++ {
			_, _ = m.InvokeTool(context.Background(), InvokeRequest{TenantID: tenantID, ServerURL: srv.URL, Auth: AuthConfig{Type: "bearer", SecretRef: "k"}, ToolName: "x", Arguments: []byte(`{}`), TimeoutMS: 1000})
		}
		_, err := m.InvokeTool(context.Background(), InvokeRequest{TenantID: tenantID, ServerURL: srv.URL, Auth: AuthConfig{Type: "bearer", SecretRef: "k"}, ToolName: "x", Arguments: []byte(`{}`), TimeoutMS: 1000})
		if err == nil || !strings.Contains(err.Error(), "circuit breaker open") {
			t.Fatalf("expected circuit open error, got %v", err)
		}
	})
}
