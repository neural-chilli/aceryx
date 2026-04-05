package invokers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/mcp"
)

type MCPManager interface {
	InvokeTool(ctx context.Context, req mcp.InvokeRequest) (mcp.MCPToolResult, error)
	DiscoverTools(ctx context.Context, tenantID uuid.UUID, serverURL string, auth mcp.AuthConfig) ([]mcp.MCPTool, error)
}

type MCPInvoker struct {
	mcpManager MCPManager
	tenantID   uuid.UUID
	serverURL  string
	toolName   string
	auth       mcp.AuthConfig
	depth      int
}

func NewMCPInvoker(mcpManager MCPManager, tenantID uuid.UUID, serverURL, toolName string, auth mcp.AuthConfig, depth int) *MCPInvoker {
	return &MCPInvoker{
		mcpManager: mcpManager,
		tenantID:   tenantID,
		serverURL:  serverURL,
		toolName:   toolName,
		auth:       auth,
		depth:      depth,
	}
}

func (mi *MCPInvoker) Invoke(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if mi == nil || mi.mcpManager == nil {
		return nil, fmt.Errorf("mcp invoker not configured")
	}
	timeoutMS := 0
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			remaining = time.Millisecond
		}
		timeoutMS = int(remaining.Milliseconds())
	}
	result, err := mi.mcpManager.InvokeTool(ctx, mcp.InvokeRequest{
		TenantID:  mi.tenantID,
		ServerURL: mi.serverURL,
		Auth:      mi.auth,
		ToolName:  mi.toolName,
		Arguments: args,
		Depth:     mi.depth,
		TimeoutMS: timeoutMS,
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("%s", mcp.ToolErrorMessage(result))
	}
	out, err := json.Marshal(result.Content)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp result: %w", err)
	}
	return out, nil
}
