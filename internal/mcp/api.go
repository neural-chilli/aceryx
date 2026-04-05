package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type API struct {
	manager *Manager
	cache   *ToolCache
}

func NewAPI(manager *Manager, cache *ToolCache) *API {
	return &API{manager: manager, cache: cache}
}

type DiscoverRequest struct {
	ServerURL  string `json:"server_url"`
	AuthType   string `json:"auth_type"`
	AuthSecret string `json:"auth_secret,omitempty"`
	HeaderName string `json:"header_name,omitempty"`
}

type RefreshRequest struct {
	ServerURL  string `json:"server_url"`
	AuthType   string `json:"auth_type,omitempty"`
	AuthSecret string `json:"auth_secret,omitempty"`
	HeaderName string `json:"header_name,omitempty"`
}

func (a *API) Discover(ctx context.Context, tenantID uuid.UUID, req DiscoverRequest) ([]MCPTool, error) {
	if a == nil || a.manager == nil {
		return nil, fmt.Errorf("mcp api not configured")
	}
	if strings.TrimSpace(req.ServerURL) == "" {
		return nil, fmt.Errorf("server_url is required")
	}
	return a.manager.DiscoverTools(ctx, tenantID, req.ServerURL, AuthConfig{
		Type:       req.AuthType,
		SecretRef:  req.AuthSecret,
		HeaderName: req.HeaderName,
	})
}

func (a *API) List(ctx context.Context, tenantID uuid.UUID) ([]CachedServer, error) {
	if a == nil || a.cache == nil {
		return nil, nil
	}
	return a.cache.ListServers(ctx, tenantID)
}

func (a *API) Delete(ctx context.Context, tenantID uuid.UUID, serverURL string) error {
	if a == nil || a.cache == nil {
		return nil
	}
	if strings.TrimSpace(serverURL) == "" {
		return fmt.Errorf("server_url is required")
	}
	return a.cache.Delete(ctx, tenantID, serverURL)
}

func (a *API) Refresh(ctx context.Context, tenantID uuid.UUID, req RefreshRequest) ([]MCPTool, error) {
	if a == nil || a.manager == nil || a.cache == nil {
		return nil, fmt.Errorf("mcp api not configured")
	}
	if strings.TrimSpace(req.ServerURL) == "" {
		return nil, fmt.Errorf("server_url is required")
	}
	_ = a.cache.MarkStale(ctx, tenantID, req.ServerURL)
	return a.manager.DiscoverTools(ctx, tenantID, req.ServerURL, AuthConfig{
		Type:       req.AuthType,
		SecretRef:  req.AuthSecret,
		HeaderName: req.HeaderName,
	})
}
