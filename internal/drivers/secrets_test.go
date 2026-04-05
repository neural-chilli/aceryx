package drivers

import (
	"context"
	"errors"
	"testing"
)

type fakeSecretStore struct {
	values map[string]string
}

func (s fakeSecretStore) Get(_ context.Context, _ string, key string) (string, error) {
	v, ok := s.values[key]
	if !ok {
		return "", errors.New("secret not found")
	}
	return v, nil
}

type secretConfig struct {
	Password  string `secret:"true"`
	APISecret string
}

func TestResolveSecrets(t *testing.T) {
	cfg := secretConfig{Password: "db-pass", APISecret: "api-key"}
	store := fakeSecretStore{values: map[string]string{"db-pass": "resolved-db", "api-key": "resolved-api"}}
	if err := ResolveSecrets(context.Background(), "tenant-1", store, &cfg); err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if cfg.Password != "resolved-db" {
		t.Fatalf("expected resolved password, got %q", cfg.Password)
	}
	if cfg.APISecret != "resolved-api" {
		t.Fatalf("expected resolved APISecret, got %q", cfg.APISecret)
	}
}

func TestResolveSecretsMissing(t *testing.T) {
	cfg := secretConfig{Password: "missing"}
	store := fakeSecretStore{values: map[string]string{}}
	if err := ResolveSecrets(context.Background(), "tenant-1", store, &cfg); err == nil {
		t.Fatal("expected missing secret error")
	}
}
