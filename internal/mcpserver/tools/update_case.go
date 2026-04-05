package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type UpdateCaseTool struct{ Store mcpserver.CaseStore }

func (t *UpdateCaseTool) Name() string               { return "update_case" }
func (t *UpdateCaseTool) RequiredPermission() string { return "cases:update" }
func (t *UpdateCaseTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{
		Name:        t.Name(),
		Description: "Update case data fields. Merges with existing data.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"case_id":{"type":"string"},"data":{"type":"object"}},"required":["case_id","data"]}`),
	}
}
func (t *UpdateCaseTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Store == nil {
		return nil, fmt.Errorf("case store not configured")
	}
	var in struct {
		CaseID string         `json:"case_id"`
		Data   map[string]any `json:"data"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	cid, err := uuid.Parse(in.CaseID)
	if err != nil {
		return nil, fmt.Errorf("invalid case_id: %w", err)
	}
	if in.Data == nil {
		return nil, fmt.Errorf("data is required")
	}
	if err := t.Store.UpdateCaseMCP(ctx, conn.TenantID, conn.UserID, cid, in.Data); err != nil {
		return nil, err
	}
	return map[string]any{"case_id": cid.String(), "status": "updated"}, nil
}
