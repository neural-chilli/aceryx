package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type ListTasksTool struct{ Store mcpserver.TaskStore }

func (t *ListTasksTool) Name() string               { return "list_tasks" }
func (t *ListTasksTool) RequiredPermission() string { return "tasks:read" }
func (t *ListTasksTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{
		Name:        t.Name(),
		Description: "List pending human tasks, optionally filtered by role or case.",
		InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"role\":{\"type\":\"string\"},\"case_id\":{\"type\":\"string\"},\"status\":{\"type\":\"string\"},\"limit\":{\"type\":\"integer\",\"default\":20}}}"),
	}
}

func (t *ListTasksTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Store == nil {
		return nil, fmt.Errorf("task store not configured")
	}
	var in struct {
		Role   string `json:"role"`
		CaseID string `json:"case_id"`
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	input := mcpserver.TaskListInput{Role: in.Role, Status: in.Status, Limit: in.Limit}
	if in.CaseID != "" {
		parsed, err := uuid.Parse(in.CaseID)
		if err != nil {
			return nil, fmt.Errorf("invalid case_id: %w", err)
		}
		input.CaseID = &parsed
	}
	items, err := t.Store.ListTasksMCP(ctx, conn.TenantID, input)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"task_id":      item.TaskID,
			"case_id":      item.CaseID.String(),
			"type":         item.Type,
			"status":       item.Status,
			"assigned_to":  item.AssignedTo,
			"sla":          item.SLA,
			"created_at":   item.CreatedAt,
			"role":         item.Role,
			"case_number":  item.CaseNumber,
			"case_type":    item.CaseType,
			"display_name": item.DisplayName,
		})
	}
	return map[string]any{"tasks": out}, nil
}
