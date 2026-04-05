package mcpserver

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultListenAddr           = ":8081"
	DefaultAuthHeader           = "X-API-Key"
	DefaultRequestsPerMinute    = 120
	DefaultMaxDepth             = 3
	DefaultMaxToolTimeout       = 120 * time.Second
	DefaultServerRequestTimeout = 30 * time.Second
)

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	ID      json.RawMessage `json:"id"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Connection struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	Roles    []string
	APIKeyID uuid.UUID
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type ToolHandler interface {
	Name() string
	Definition() ToolDefinition
	RequiredPermission() string
	Execute(ctx context.Context, conn *Connection, args json.RawMessage) (any, error)
}

type RateLimitConfig struct {
	RequestsPerMinute int            `json:"requests_per_minute"`
	ToolLimits        map[string]int `json:"tool_limits,omitempty"`
}

type ServerConfig struct {
	Enabled           bool            `json:"enabled"`
	ListenAddr        string          `json:"listen_addr"`
	AuthType          string          `json:"auth_type"`
	AuthHeader        string          `json:"auth_header"`
	RateLimit         RateLimitConfig `json:"rate_limit"`
	EnabledTools      []string        `json:"enabled_tools,omitempty"`
	DisabledTools     []string        `json:"disabled_tools,omitempty"`
	MaxDepth          int             `json:"max_depth"`
	MaxRequestTimeout time.Duration   `json:"max_request_timeout"`
}

type ServerDependencies struct {
	Tools       []ToolHandler
	AuditStore  InvocationStore
	APIKeyStore APIKeyStore
}

func (cfg ServerConfig) WithDefaults() ServerConfig {
	out := cfg
	if out.ListenAddr == "" {
		out.ListenAddr = DefaultListenAddr
	}
	if out.AuthType == "" {
		out.AuthType = "api_key"
	}
	if out.AuthHeader == "" {
		out.AuthHeader = DefaultAuthHeader
	}
	if out.RateLimit.RequestsPerMinute <= 0 {
		out.RateLimit.RequestsPerMinute = DefaultRequestsPerMinute
	}
	if out.MaxDepth <= 0 {
		out.MaxDepth = DefaultMaxDepth
	}
	if out.MaxRequestTimeout <= 0 {
		out.MaxRequestTimeout = DefaultMaxToolTimeout
	}
	return out
}
