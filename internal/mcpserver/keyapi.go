package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CreateKeyInput struct {
	UserID  uuid.UUID
	Name    string
	Roles   []string
	Enabled bool
}

type UpdateKeyInput struct {
	Name    string
	Roles   []string
	Enabled bool
}

type CreatedKey struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

type KeyAPI struct {
	keys   APIKeyStore
	server *Server
}

func NewKeyAPI(keys APIKeyStore, server *Server) *KeyAPI {
	return &KeyAPI{keys: keys, server: server}
}

func (api *KeyAPI) List(ctx context.Context, tenantID uuid.UUID) ([]*APIKeyRecord, error) {
	if api == nil || api.keys == nil {
		return nil, fmt.Errorf("mcp key api not configured")
	}
	return api.keys.List(ctx, tenantID)
}

func (api *KeyAPI) Create(ctx context.Context, tenantID uuid.UUID, in CreateKeyInput) (*CreatedKey, error) {
	if api == nil || api.keys == nil {
		return nil, fmt.Errorf("mcp key api not configured")
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if in.UserID == uuid.Nil {
		return nil, fmt.Errorf("user_id is required")
	}
	rec := &APIKeyRecord{TenantID: tenantID, UserID: in.UserID, Name: strings.TrimSpace(in.Name), Roles: append([]string(nil), in.Roles...), Enabled: in.Enabled}
	if !in.Enabled {
		// explicit false
	} else {
		rec.Enabled = true
	}
	raw, err := api.keys.Create(ctx, rec)
	if err != nil {
		return nil, err
	}
	return &CreatedKey{ID: rec.ID, Name: rec.Name, Key: raw, Roles: rec.Roles, CreatedAt: rec.CreatedAt}, nil
}

func (api *KeyAPI) Update(ctx context.Context, tenantID, id uuid.UUID, in UpdateKeyInput) (*APIKeyRecord, error) {
	if api == nil || api.keys == nil {
		return nil, fmt.Errorf("mcp key api not configured")
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	return api.keys.Update(ctx, tenantID, id, strings.TrimSpace(in.Name), in.Roles, in.Enabled)
}

func (api *KeyAPI) Revoke(ctx context.Context, tenantID, id uuid.UUID) error {
	if api == nil || api.keys == nil {
		return fmt.Errorf("mcp key api not configured")
	}
	return api.keys.Revoke(ctx, tenantID, id)
}

func (api *KeyAPI) GetConfig() ServerConfig {
	if api == nil || api.server == nil {
		return ServerConfig{}.WithDefaults()
	}
	return api.server.Config()
}

func (api *KeyAPI) UpdateConfig(ctx context.Context, cfg ServerConfig) (ServerConfig, error) {
	if api == nil || api.server == nil {
		return ServerConfig{}, fmt.Errorf("mcp server not configured")
	}
	old := api.server.Config()
	newCfg := cfg.WithDefaults()
	api.server.SetConfig(newCfg)

	restart := old.ListenAddr != newCfg.ListenAddr
	wasEnabled := old.Enabled
	isEnabled := newCfg.Enabled
	if wasEnabled && !isEnabled {
		if err := api.server.Stop(ctx); err != nil {
			return old, err
		}
		return newCfg, nil
	}
	if !wasEnabled && isEnabled {
		if err := api.server.Start(ctx); err != nil {
			return old, err
		}
		return newCfg, nil
	}
	if isEnabled && restart {
		if err := api.server.Stop(ctx); err != nil {
			return old, err
		}
		if err := api.server.Start(ctx); err != nil {
			return old, err
		}
	}
	return newCfg, nil
}
