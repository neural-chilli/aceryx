package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
	"github.com/neural-chilli/aceryx/internal/llm"
)

type cbKey struct {
	TenantID  string
	ServerURL string
}

type Manager struct {
	cache           *ToolCache
	clientFactory   func(serverURL string, auth AuthConfig) *Client
	circuitBreakers map[cbKey]*CircuitBreaker
	secretStore     connectors.SecretStore
	selfURLs        []string
	maxDepth        int
	mu              sync.RWMutex
}

func NewManager(cache *ToolCache, secretStore connectors.SecretStore, selfURLs []string, httpClient *http.Client) *Manager {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: defaultRequestTimeout}
	}
	return &Manager{
		cache:       cache,
		secretStore: secretStore,
		selfURLs:    append([]string(nil), selfURLs...),
		maxDepth:    DefaultMaxDepth,
		clientFactory: func(serverURL string, auth AuthConfig) *Client {
			return NewClient(serverURL, client, auth)
		},
		circuitBreakers: map[cbKey]*CircuitBreaker{},
	}
}

func (m *Manager) DiscoverTools(ctx context.Context, tenantID uuid.UUID, serverURL string, auth AuthConfig) ([]MCPTool, error) {
	if m == nil {
		return nil, fmt.Errorf("mcp manager not configured")
	}
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return nil, fmt.Errorf("server_url is required")
	}
	if err := m.CheckSelfInvocation(serverURL); err != nil {
		return nil, err
	}
	if m.cache != nil {
		cached, err := m.cache.GetTools(ctx, tenantID, serverURL)
		if err != nil {
			return nil, err
		}
		if len(cached) > 0 {
			return cached, nil
		}
	}
	resolvedAuth, err := m.resolveAuth(ctx, tenantID, auth)
	if err != nil {
		return nil, err
	}
	tools, err := m.clientFactory(serverURL, resolvedAuth).Discover(ctx)
	if err != nil {
		if m.cache != nil {
			_ = m.cache.SetError(ctx, tenantID, serverURL, err)
		}
		return nil, err
	}
	if m.cache != nil {
		if err := m.cache.SetTools(ctx, tenantID, serverURL, tools); err != nil {
			return nil, err
		}
	}
	return tools, nil
}

func (m *Manager) InvokeTool(ctx context.Context, req InvokeRequest) (MCPToolResult, error) {
	if m == nil {
		return MCPToolResult{}, fmt.Errorf("mcp manager not configured")
	}
	if err := m.CheckSelfInvocation(req.ServerURL); err != nil {
		return MCPToolResult{}, err
	}
	depth := req.Depth
	if depth <= 0 {
		depth = getDepthFromContext(ctx)
	}
	if err := CheckDepth(depth, m.maxDepth); err != nil {
		return MCPToolResult{}, err
	}
	cb := m.getCircuitBreaker(req.TenantID, req.ServerURL)
	if err := cb.Allow(); err != nil {
		return MCPToolResult{}, fmt.Errorf("MCP server %s circuit breaker open — recent failures", req.ServerURL)
	}
	resolvedAuth, err := m.resolveAuth(ctx, req.TenantID, req.Auth)
	if err != nil {
		cb.RecordFailure()
		return MCPToolResult{}, err
	}
	callCtx, cancel := CascadeTimeout(WithDepth(ctx, depth), req.TimeoutMS)
	defer cancel()
	result, err := m.clientFactory(req.ServerURL, resolvedAuth).Invoke(callCtx, req.ToolName, req.Arguments)
	if err != nil {
		cb.RecordFailure()
		if m.cache != nil {
			_ = m.cache.MarkStale(ctx, req.TenantID, req.ServerURL)
		}
		return MCPToolResult{}, err
	}
	cb.RecordSuccess()
	return result, nil
}

func (m *Manager) ToolsForAgent(ctx context.Context, tenantID uuid.UUID, serverURL, prefix string, toolFilter []string) ([]llm.ToolDef, error) {
	tools, err := m.DiscoverTools(ctx, tenantID, serverURL, AuthConfig{Type: "none"})
	if err != nil {
		return nil, err
	}
	return ToLLMToolDefs(tools, prefix, toolFilter), nil
}

func (m *Manager) getCircuitBreaker(tenantID uuid.UUID, serverURL string) *CircuitBreaker {
	key := cbKey{TenantID: tenantID.String(), ServerURL: strings.TrimSpace(serverURL)}
	m.mu.RLock()
	cb, ok := m.circuitBreakers[key]
	m.mu.RUnlock()
	if ok {
		return cb
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.circuitBreakers[key]; ok {
		return existing
	}
	created := NewCircuitBreaker(60*time.Second, 5, 5*time.Minute)
	m.circuitBreakers[key] = created
	return created
}

func (m *Manager) resolveAuth(ctx context.Context, tenantID uuid.UUID, cfg AuthConfig) (AuthConfig, error) {
	resolved := cfg
	typ := strings.ToLower(strings.TrimSpace(cfg.Type))
	if typ == "" || typ == "none" {
		resolved.Type = "none"
		resolved.SecretRef = ""
		return resolved, nil
	}
	if strings.TrimSpace(cfg.SecretRef) == "" {
		return AuthConfig{}, fmt.Errorf("auth_secret is required for auth_type %s", cfg.Type)
	}
	if m.secretStore == nil {
		return AuthConfig{}, fmt.Errorf("secret store unavailable")
	}
	value, err := m.secretStore.Get(ctx, tenantID, cfg.SecretRef)
	if err != nil {
		return AuthConfig{}, fmt.Errorf("resolve auth secret %s: %w", cfg.SecretRef, err)
	}
	resolved.Type = typ
	resolved.SecretRef = value
	if typ == "api_key" && strings.TrimSpace(resolved.HeaderName) == "" {
		resolved.HeaderName = "X-API-Key"
	}
	return resolved, nil
}

func ToolErrorMessage(result MCPToolResult) string {
	if !result.IsError {
		return ""
	}
	parts := make([]string, 0, len(result.Content))
	for _, c := range result.Content {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(c.Text))
	}
	if len(parts) == 0 {
		return "MCP tool returned an error"
	}
	return strings.Join(parts, "\n")
}

func MarshalAny(v any) (json.RawMessage, error) {
	if v == nil {
		return json.RawMessage([]byte("{}")), nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}
