package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type SearchCasesTool struct{ Store mcpserver.CaseStore }

func (t *SearchCasesTool) Name() string               { return "search_cases" }
func (t *SearchCasesTool) RequiredPermission() string { return "cases:read" }
func (t *SearchCasesTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{
		Name:        t.Name(),
		Description: "Search cases by type, status, data fields, or date range.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"case_type":{"type":"string"},"status":{"type":"string"},"filters":{"type":"object"},"since":{"type":"string","format":"date-time"},"limit":{"type":"integer","default":20}}}`),
	}
}
func (t *SearchCasesTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Store == nil {
		return nil, fmt.Errorf("case store not configured")
	}
	var in struct {
		CaseType string         `json:"case_type"`
		Status   string         `json:"status"`
		Filters  map[string]any `json:"filters"`
		Since    string         `json:"since"`
		Limit    int            `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	var since *time.Time
	if in.Since != "" {
		parsed, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid since datetime: %w", err)
		}
		since = &parsed
	}
	results, total, err := t.Store.SearchCasesMCP(ctx, conn.TenantID, mcpserver.CaseSearchInput{CaseType: in.CaseType, Status: in.Status, Filters: in.Filters, Since: since, Limit: in.Limit})
	if err != nil {
		return nil, err
	}
	payload := make([]map[string]any, 0, len(results))
	for _, item := range results {
		payload = append(payload, map[string]any{"case_id": item.CaseID.String(), "status": item.Status, "summary": item.Summary, "created_at": item.CreatedAt, "case_type": item.CaseType, "case_number": item.CaseNumber})
	}
	return map[string]any{"cases": payload, "total_count": total}, nil
}
