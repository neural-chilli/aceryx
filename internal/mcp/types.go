package mcp

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AuthConfig struct {
	Type       string `json:"type"`
	SecretRef  string `json:"secret_ref,omitempty"`
	HeaderName string `json:"header_name,omitempty"`
}

type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError"`
}

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type CachedServer struct {
	TenantID          uuid.UUID     `json:"tenant_id"`
	ServerURL         string        `json:"server_url"`
	Tools             []MCPTool     `json:"tools"`
	LastDiscovered    time.Time     `json:"last_discovered"`
	Status            string        `json:"status"`
	ErrorMessage      string        `json:"error_message,omitempty"`
	LastDiscoveredAge time.Duration `json:"-"`
}

type InvokeRequest struct {
	TenantID  uuid.UUID
	ServerURL string
	Auth      AuthConfig
	ToolName  string
	Arguments json.RawMessage
	Depth     int
	TimeoutMS int
}
