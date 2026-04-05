package mcpserver

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type mockKeyStore struct {
	rec *APIKeyRecord
	err error
}

func (m *mockKeyStore) ValidateKey(context.Context, string) (*APIKeyRecord, error) {
	return m.rec, m.err
}
func (m *mockKeyStore) Create(context.Context, *APIKeyRecord) (string, error)    { return "", nil }
func (m *mockKeyStore) List(context.Context, uuid.UUID) ([]*APIKeyRecord, error) { return nil, nil }
func (m *mockKeyStore) Update(context.Context, uuid.UUID, uuid.UUID, string, []string, bool) (*APIKeyRecord, error) {
	return nil, nil
}
func (m *mockKeyStore) Revoke(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func TestAuthenticateValid(t *testing.T) {
	store := &mockKeyStore{rec: &APIKeyRecord{ID: uuid.New(), TenantID: uuid.New(), UserID: uuid.New(), Roles: []string{"cases:read"}, Enabled: true}}
	auth := NewAuthMiddleware(store, ServerConfig{})
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-API-Key", "secret")
	conn, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if conn.TenantID == uuid.Nil || conn.UserID == uuid.Nil {
		t.Fatalf("expected tenant and user ids")
	}
}

func TestAuthenticateInvalid(t *testing.T) {
	store := &mockKeyStore{err: sql.ErrNoRows}
	auth := NewAuthMiddleware(store, ServerConfig{})
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-API-Key", "secret")
	if _, err := auth.Authenticate(req); err == nil {
		t.Fatalf("expected invalid auth error")
	}
}
