package invokers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/mcp"
)

type mcpManagerMock struct{}

func (m mcpManagerMock) InvokeTool(context.Context, mcp.InvokeRequest) (mcp.MCPToolResult, error) {
	return mcp.MCPToolResult{Content: []mcp.MCPContent{{Type: "text", Text: "ok"}}}, nil
}
func (m mcpManagerMock) DiscoverTools(context.Context, uuid.UUID, string, mcp.AuthConfig) ([]mcp.MCPTool, error) {
	return nil, nil
}

func TestMCPInvoker_Invoke(t *testing.T) {
	inv := NewMCPInvoker(mcpManagerMock{}, uuid.New(), "https://example.com", "search", mcp.AuthConfig{Type: "none"}, 1)
	out, err := inv.Invoke(context.Background(), json.RawMessage(`{"q":"x"}`))
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected result")
	}
}
