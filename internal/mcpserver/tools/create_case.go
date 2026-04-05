package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type CreateCaseTool struct {
	Store mcpserver.CaseStore
}

func (t *CreateCaseTool) Name() string               { return "create_case" }
func (t *CreateCaseTool) RequiredPermission() string { return "cases:create" }
func (t *CreateCaseTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{
		Name:        t.Name(),
		Description: "Create a new case in Aceryx. Optionally triggers the associated workflow.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"case_type":{"type":"string"},"data":{"type":"object"},"trigger_workflow":{"type":"boolean","default":true}},"required":["case_type","data"]}`),
	}
}

func (t *CreateCaseTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Store == nil {
		return nil, fmt.Errorf("case store not configured")
	}
	var in struct {
		CaseType        string         `json:"case_type"`
		Data            map[string]any `json:"data"`
		TriggerWorkflow *bool          `json:"trigger_workflow,omitempty"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if in.CaseType == "" {
		return nil, fmt.Errorf("case_type is required")
	}
	if in.Data == nil {
		return nil, fmt.Errorf("data is required")
	}
	trigger := true
	if in.TriggerWorkflow != nil {
		trigger = *in.TriggerWorkflow
	}
	caseID, status, err := t.Store.CreateCaseMCP(ctx, conn.TenantID, conn.UserID, in.CaseType, in.Data, trigger)
	if err != nil {
		return nil, err
	}
	return map[string]any{"case_id": caseID.String(), "status": status}, nil
}
