package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type GetCaseTool struct {
	Store mcpserver.CaseStore
}

func (t *GetCaseTool) Name() string               { return "get_case" }
func (t *GetCaseTool) RequiredPermission() string { return "cases:read" }
func (t *GetCaseTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{
		Name:        t.Name(),
		Description: "Retrieve a case by ID, including current data, status, and timeline.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"case_id":{"type":"string"}},"required":["case_id"]}`),
	}
}

func (t *GetCaseTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Store == nil {
		return nil, fmt.Errorf("case store not configured")
	}
	var in struct {
		CaseID string `json:"case_id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	caseID, err := uuid.Parse(in.CaseID)
	if err != nil {
		return nil, fmt.Errorf("invalid case_id: %w", err)
	}
	item, err := t.Store.GetCaseMCP(ctx, conn.TenantID, caseID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"case_id":     item.CaseID.String(),
		"status":      item.Status,
		"data":        item.Data,
		"created_at":  item.CreatedAt,
		"updated_at":  item.UpdatedAt,
		"timeline":    item.Timeline,
		"case_type":   item.CaseType,
		"case_number": item.CaseNumber,
	}, nil
}
