package hostfns

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
)

type fakeSecrets struct {
	values map[string]string
}

func (f *fakeSecrets) Get(_ context.Context, _ uuid.UUID, key string) (string, error) {
	v, ok := f.values[key]
	if !ok {
		return "", connectors.ErrSecretNotFound
	}
	return v, nil
}

func TestSecretGet(t *testing.T) {
	host := &SecretGetter{
		Store: &fakeSecrets{values: map[string]string{"api_key": "abc"}},
	}
	v, err := host.SecretGet("api_key")
	if err != nil {
		t.Fatalf("SecretGet error: %v", err)
	}
	if v != "abc" {
		t.Fatalf("unexpected value: %s", v)
	}
}

func TestSecretGetMissing(t *testing.T) {
	host := &SecretGetter{
		Store: &fakeSecrets{values: map[string]string{}},
	}
	_, err := host.SecretGet("missing")
	if err == nil {
		t.Fatal("expected missing secret error")
	}
	if errors.Is(err, connectors.ErrSecretNotFound) {
		t.Fatal("expected wrapped user-facing error, not raw connector error")
	}
}
