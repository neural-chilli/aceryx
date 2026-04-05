package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type ListCaseTypesTool struct{ Store mcpserver.CaseTypeStore }

func (t *ListCaseTypesTool) Name() string               { return "list_case_types" }
func (t *ListCaseTypesTool) RequiredPermission() string { return "cases:read" }
func (t *ListCaseTypesTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{Name: t.Name(), Description: "List available case types with their schemas.", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}
}
func (t *ListCaseTypesTool) Execute(ctx context.Context, conn *mcpserver.Connection, _ json.RawMessage) (any, error) {
	if t.Store == nil {
		return nil, fmt.Errorf("case type store not configured")
	}
	types, err := t.Store.ListCaseTypes(ctx, conn.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(types))
	for _, ct := range types {
		out = append(out, map[string]any{"id": ct.ID.String(), "name": ct.Name, "schema": ct.Schema})
	}
	return map[string]any{"case_types": out}, nil
}
