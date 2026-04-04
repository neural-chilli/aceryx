package hostfns

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
)

type SecretGetter struct {
	Store    connectors.SecretStore
	TenantID uuid.UUID
}

func (s *SecretGetter) SecretGet(key string) (string, error) {
	if s.Store == nil {
		return "", fmt.Errorf("secret not found: %s", key)
	}
	value, err := s.Store.Get(context.Background(), s.TenantID, key)
	if err != nil {
		return "", fmt.Errorf("secret not found: %s", key)
	}
	if value == "" {
		return "", fmt.Errorf("secret not found: %s", key)
	}
	return value, nil
}
