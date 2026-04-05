package mcpserver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")

type APIKeyRecord struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	UserID     uuid.UUID  `json:"user_id"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	Roles      []string   `json:"roles"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type APIKeyStore interface {
	ValidateKey(ctx context.Context, key string) (*APIKeyRecord, error)
	Create(ctx context.Context, record *APIKeyRecord) (string, error)
	List(ctx context.Context, tenantID uuid.UUID) ([]*APIKeyRecord, error)
	Update(ctx context.Context, tenantID, id uuid.UUID, name string, roles []string, enabled bool) (*APIKeyRecord, error)
	Revoke(ctx context.Context, tenantID, id uuid.UUID) error
}

type AuthMiddleware struct {
	keyStore APIKeyStore
	config   ServerConfig
}

func NewAuthMiddleware(keyStore APIKeyStore, cfg ServerConfig) *AuthMiddleware {
	return &AuthMiddleware{keyStore: keyStore, config: cfg.WithDefaults()}
}

func (am *AuthMiddleware) Authenticate(r *http.Request) (*Connection, error) {
	if am == nil || am.keyStore == nil {
		return nil, fmt.Errorf("%w: key store not configured", ErrUnauthorized)
	}
	if r == nil {
		return nil, fmt.Errorf("%w: request is nil", ErrUnauthorized)
	}

	cfg := am.config.WithDefaults()
	raw := ""
	switch strings.ToLower(strings.TrimSpace(cfg.AuthType)) {
	case "bearer":
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			return nil, fmt.Errorf("%w: missing bearer token", ErrUnauthorized)
		}
		raw = strings.TrimSpace(authz[len("Bearer "):])
	default:
		raw = strings.TrimSpace(r.Header.Get(cfg.AuthHeader))
	}
	if raw == "" {
		return nil, fmt.Errorf("%w: missing API key", ErrUnauthorized)
	}

	rec, err := am.keyStore.ValidateKey(r.Context(), raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: invalid API key", ErrUnauthorized)
		}
		return nil, err
	}
	if rec == nil || !rec.Enabled {
		return nil, fmt.Errorf("%w: key disabled", ErrUnauthorized)
	}
	return &Connection{
		TenantID: rec.TenantID,
		UserID:   rec.UserID,
		Roles:    append([]string(nil), rec.Roles...),
		APIKeyID: rec.ID,
	}, nil
}

func GenerateAPIKey() (raw string, hash string, err error) {
	buf := make([]byte, 16)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate random key bytes: %w", err)
	}
	raw = "aceryx_mcp_" + hex.EncodeToString(buf)
	hashed, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		return "", "", fmt.Errorf("hash key: %w", err)
	}
	return raw, string(hashed), nil
}

type PostgresAPIKeyStore struct {
	db *sql.DB
}

func NewPostgresAPIKeyStore(db *sql.DB) *PostgresAPIKeyStore {
	return &PostgresAPIKeyStore{db: db}
}

func (s *PostgresAPIKeyStore) ValidateKey(ctx context.Context, key string) (*APIKeyRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("api key store not configured")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, user_id, name, key_hash, roles, enabled, created_at, last_used_at
FROM mcp_api_keys
WHERE enabled = TRUE
`)
	if err != nil {
		return nil, fmt.Errorf("query mcp keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		rec, scanErr := scanAPIKeyRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if bcrypt.CompareHashAndPassword([]byte(rec.KeyHash), []byte(key)) == nil {
			_, _ = s.db.ExecContext(ctx, `UPDATE mcp_api_keys SET last_used_at = now() WHERE id = $1`, rec.ID)
			return rec, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mcp keys: %w", err)
	}
	return nil, sql.ErrNoRows
}

func (s *PostgresAPIKeyStore) Create(ctx context.Context, record *APIKeyRecord) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("api key store not configured")
	}
	if record == nil {
		return "", fmt.Errorf("record is required")
	}
	raw, hash, err := GenerateAPIKey()
	if err != nil {
		return "", err
	}
	rolesJSON, err := json.Marshal(record.Roles)
	if err != nil {
		return "", fmt.Errorf("marshal key roles: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
INSERT INTO mcp_api_keys (tenant_id, user_id, name, key_hash, roles, enabled)
VALUES ($1, $2, $3, $4, $5::jsonb, COALESCE($6, TRUE))
RETURNING id, created_at
`, record.TenantID, record.UserID, strings.TrimSpace(record.Name), hash, string(rolesJSON), record.Enabled).Scan(&record.ID, &record.CreatedAt); err != nil {
		return "", fmt.Errorf("insert mcp key: %w", err)
	}
	record.KeyHash = hash
	if !record.Enabled {
		// enabled explicitly false should remain false
	} else {
		record.Enabled = true
	}
	return raw, nil
}

func (s *PostgresAPIKeyStore) List(ctx context.Context, tenantID uuid.UUID) ([]*APIKeyRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("api key store not configured")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, user_id, name, key_hash, roles, enabled, created_at, last_used_at
FROM mcp_api_keys
WHERE tenant_id = $1
ORDER BY created_at DESC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list mcp keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*APIKeyRecord, 0)
	for rows.Next() {
		rec, scanErr := scanAPIKeyRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mcp keys: %w", err)
	}
	return out, nil
}

func (s *PostgresAPIKeyStore) Update(ctx context.Context, tenantID, id uuid.UUID, name string, roles []string, enabled bool) (*APIKeyRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("api key store not configured")
	}
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return nil, fmt.Errorf("marshal roles: %w", err)
	}
	row := s.db.QueryRowContext(ctx, `
UPDATE mcp_api_keys
SET name = $3,
    roles = $4::jsonb,
    enabled = $5
WHERE tenant_id = $1 AND id = $2
RETURNING id, tenant_id, user_id, name, key_hash, roles, enabled, created_at, last_used_at
`, tenantID, id, strings.TrimSpace(name), string(rolesJSON), enabled)
	rec, err := scanAPIKeyRecordRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return rec, nil
}

func (s *PostgresAPIKeyStore) Revoke(ctx context.Context, tenantID, id uuid.UUID) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("api key store not configured")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM mcp_api_keys WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return fmt.Errorf("delete mcp key: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAPIKeyRecordRow(row scanner) (*APIKeyRecord, error) {
	return scanRecord(row)
}

func scanAPIKeyRecord(rows *sql.Rows) (*APIKeyRecord, error) {
	return scanRecord(rows)
}

func scanRecord(s scanner) (*APIKeyRecord, error) {
	rec := &APIKeyRecord{}
	var rolesRaw []byte
	if err := s.Scan(&rec.ID, &rec.TenantID, &rec.UserID, &rec.Name, &rec.KeyHash, &rolesRaw, &rec.Enabled, &rec.CreatedAt, &rec.LastUsedAt); err != nil {
		return nil, err
	}
	if len(rolesRaw) > 0 {
		if err := json.Unmarshal(rolesRaw, &rec.Roles); err != nil {
			return nil, fmt.Errorf("decode mcp key roles: %w", err)
		}
	}
	return rec, nil
}
