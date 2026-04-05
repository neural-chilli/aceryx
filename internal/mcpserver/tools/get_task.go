package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type GetTaskTool struct{ Store mcpserver.TaskStore }

func (t *GetTaskTool) Name() string               { return "get_task" }
func (t *GetTaskTool) RequiredPermission() string { return "tasks:read" }
func (t *GetTaskTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{
		Name:        t.Name(),
		Description: "Get task details including form schema, case data, and agentic reasoning trace if applicable.",
		InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"task_id\":{\"type\":\"string\"}},\"required\":[\"task_id\"]}"),
	}
}

func (t *GetTaskTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Store == nil {
		return nil, fmt.Errorf("task store not configured")
	}
	var in struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if in.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	detail, err := t.Store.GetTaskMCP(ctx, conn.TenantID, in.TaskID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"task_id":           detail.TaskID,
		"case_id":           detail.CaseID.String(),
		"type":              detail.Type,
		"status":            detail.Status,
		"form_schema":       detail.FormSchema,
		"case_data":         detail.CaseData,
		"reasoning_trace":   detail.ReasoningTrace,
		"available_actions": detail.AvailableAction,
	}, nil
}
