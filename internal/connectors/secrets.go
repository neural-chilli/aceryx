package connectors

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
)

var ErrSecretNotFound = errors.New("secret not found")

type SecretStore interface {
	Get(ctx context.Context, tenantID uuid.UUID, key string) (string, error)
}

type EnvSecretStore struct{}

func (s *EnvSecretStore) Get(_ context.Context, _ uuid.UUID, key string) (string, error) {
	envKey := "ACERYX_SECRET_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	value := os.Getenv(envKey)
	if value == "" {
		return "", ErrSecretNotFound
	}
	return value, nil
}

type DBSecretStore struct {
	db *sql.DB
}

func NewDBSecretStore(db *sql.DB) *DBSecretStore {
	return &DBSecretStore{db: db}
}

func (s *DBSecretStore) Get(ctx context.Context, tenantID uuid.UUID, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `
SELECT value_encrypted
FROM secrets
WHERE tenant_id = $1 AND key = $2
`, tenantID, key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSecretNotFound
		}
		return "", fmt.Errorf("lookup secret %s: %w", key, err)
	}
	if value == "" {
		return "", ErrSecretNotFound
	}
	return value, nil
}

type ChainedSecretStore struct {
	stores []SecretStore
}

func NewChainedSecretStore(stores ...SecretStore) *ChainedSecretStore {
	return &ChainedSecretStore{stores: stores}
}

func (s *ChainedSecretStore) Get(ctx context.Context, tenantID uuid.UUID, key string) (string, error) {
	for _, store := range s.stores {
		if store == nil {
			continue
		}
		value, err := store.Get(ctx, tenantID, key)
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, ErrSecretNotFound) {
			return "", err
		}
	}
	return "", ErrSecretNotFound
}
