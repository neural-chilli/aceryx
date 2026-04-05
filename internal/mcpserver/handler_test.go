package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type handlerTool struct{}

func (handlerTool) Name() string               { return "get_case" }
func (handlerTool) RequiredPermission() string { return "cases:read" }
func (handlerTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "get_case", Description: "x", InputSchema: json.RawMessage("{}")}
}
func (handlerTool) Execute(context.Context, *Connection, json.RawMessage) (any, error) {
	return map[string]any{"ok": true}, nil
}

type fixedKeyStore struct{}

func (fixedKeyStore) ValidateKey(context.Context, string) (*APIKeyRecord, error) {
	return &APIKeyRecord{ID: uuid.New(), TenantID: uuid.New(), UserID: uuid.New(), Roles: []string{"cases:read"}, Enabled: true}, nil
}
func (fixedKeyStore) Create(context.Context, *APIKeyRecord) (string, error) { return "", nil }
func (fixedKeyStore) List(context.Context, uuid.UUID) ([]*APIKeyRecord, error) {
	return nil, nil
}
func (fixedKeyStore) Update(context.Context, uuid.UUID, uuid.UUID, string, []string, bool) (*APIKeyRecord, error) {
	return nil, nil
}
func (fixedKeyStore) Revoke(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func TestHandlerToolsList(t *testing.T) {
	h := NewHandler(ServerConfig{}, []ToolHandler{handlerTool{}}, NewAuthMiddleware(fixedKeyStore{}, ServerConfig{}), NewRateLimiter(RateLimitConfig{RequestsPerMinute: 100}), nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "k")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp JSONRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestHandlerUnknownMethod(t *testing.T) {
	h := NewHandler(ServerConfig{}, []ToolHandler{handlerTool{}}, NewAuthMiddleware(fixedKeyStore{}, ServerConfig{}), NewRateLimiter(RateLimitConfig{RequestsPerMinute: 100}), nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/unknown"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "k")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp JSONRPCResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != rpcMethodNotFound {
		t.Fatalf("expected method not found, got %+v", resp.Error)
	}
}
