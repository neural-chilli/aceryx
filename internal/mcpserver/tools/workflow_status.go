package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type WorkflowStatusTool struct{ Engine mcpserver.WorkflowEngine }

func (t *WorkflowStatusTool) Name() string               { return "get_workflow_status" }
func (t *WorkflowStatusTool) RequiredPermission() string { return "workflows:view" }
func (t *WorkflowStatusTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{Name: t.Name(), Description: "Get workflow execution status for a case.", InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"case_id\":{\"type\":\"string\"}},\"required\":[\"case_id\"]}")}
}

func (t *WorkflowStatusTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Engine == nil {
		return nil, fmt.Errorf("workflow engine not configured")
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
	status, err := t.Engine.GetStatus(ctx, conn.TenantID, caseID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"case_id": status.CaseID.String(), "workflow_id": status.WorkflowID.String(), "status": status.Status, "current_step": status.CurrentStep, "completed_steps": status.CompletedSteps, "pending_tasks": status.PendingTasks, "progress_pct": status.ProgressPct}, nil
}
