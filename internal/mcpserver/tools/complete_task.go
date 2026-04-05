package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type CompleteTaskTool struct{ Store mcpserver.TaskStore }

func (t *CompleteTaskTool) Name() string               { return "complete_task" }
func (t *CompleteTaskTool) RequiredPermission() string { return "tasks:complete" }
func (t *CompleteTaskTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{
		Name:        t.Name(),
		Description: "Complete a human task with form data. Advances the workflow.",
		InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"task_id\":{\"type\":\"string\"},\"decision\":{\"type\":\"string\"},\"form_data\":{\"type\":\"object\"},\"notes\":{\"type\":\"string\"}},\"required\":[\"task_id\",\"decision\"]}"),
	}
}

func (t *CompleteTaskTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Store == nil {
		return nil, fmt.Errorf("task store not configured")
	}
	var in struct {
		TaskID   string         `json:"task_id"`
		Decision string         `json:"decision"`
		FormData map[string]any `json:"form_data"`
		Notes    string         `json:"notes"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if in.TaskID == "" || in.Decision == "" {
		return nil, fmt.Errorf("task_id and decision are required")
	}
	result, err := t.Store.CompleteTaskMCP(ctx, conn.TenantID, conn.UserID, mcpserver.TaskCompleteInput{TaskID: in.TaskID, Decision: in.Decision, FormData: in.FormData, Notes: in.Notes})
	if err != nil {
		return nil, err
	}
	return map[string]any{"task_id": result.TaskID, "status": result.Status, "case_id": result.CaseID.String()}, nil
}
