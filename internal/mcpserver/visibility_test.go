package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeTool struct {
	name string
	perm string
}

func (t fakeTool) Name() string               { return t.name }
func (t fakeTool) RequiredPermission() string { return t.perm }
func (t fakeTool) Definition() ToolDefinition {
	return ToolDefinition{Name: t.name, InputSchema: json.RawMessage("{}")}
}
func (t fakeTool) Execute(context.Context, *Connection, json.RawMessage) (any, error) {
	return nil, nil
}

func TestVisibleToolsFilters(t *testing.T) {
	h := NewHandler(ServerConfig{DisabledTools: []string{"update_case"}}, []ToolHandler{fakeTool{name: "update_case", perm: "cases:update"}, fakeTool{name: "get_case", perm: "cases:read"}}, nil, nil, nil)
	tools := h.visibleTools(&Connection{Roles: []string{"cases:read"}})
	if len(tools) != 1 || tools[0].Name != "get_case" {
		t.Fatalf("unexpected visible tools: %+v", tools)
	}
}
