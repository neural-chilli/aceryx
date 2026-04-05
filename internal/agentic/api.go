package agentic

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type API struct {
	db    *sql.DB
	store TraceStore
}

func NewAPI(db *sql.DB, store TraceStore) *API {
	if store == nil {
		store = NewPostgresTraceStore(db)
	}
	return &API{db: db, store: store}
}

func (a *API) ListByCase(ctx context.Context, tenantID, caseID uuid.UUID) ([]*ReasoningTrace, error) {
	if a == nil || a.db == nil {
		return nil, nil
	}
	rows, err := a.db.QueryContext(ctx, `
SELECT id, tenant_id, case_id, step_id, instance_id, model_used, goal, status,
       COALESCE(conclusion, '{}'::jsonb), total_iterations, total_tool_calls, total_tokens,
       COALESCE(total_duration_ms, 0), created_at, completed_at
FROM agentic_reasoning_traces
WHERE tenant_id = $1 AND case_id = $2
ORDER BY created_at DESC
`, tenantID, caseID)
	if err != nil {
		return nil, fmt.Errorf("list traces by case: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]*ReasoningTrace, 0)
	for rows.Next() {
		var t ReasoningTrace
		if err := rows.Scan(&t.ID, &t.TenantID, &t.CaseID, &t.StepID, &t.InstanceID, &t.ModelUsed, &t.Goal, &t.Status,
			&t.Conclusion, &t.TotalIterations, &t.TotalToolCalls, &t.TotalTokens, &t.TotalDurationMS, &t.CreatedAt, &t.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (a *API) GetTrace(ctx context.Context, tenantID, traceID uuid.UUID) (*ReasoningTrace, error) {
	t, err := a.store.GetTrace(ctx, traceID)
	if err != nil {
		return nil, err
	}
	if t.TenantID != tenantID {
		return nil, sql.ErrNoRows
	}
	return t, nil
}

func (a *API) GetEvents(ctx context.Context, tenantID, traceID uuid.UUID, eventType string) ([]*ReasoningEvent, error) {
	trace, err := a.GetTrace(ctx, tenantID, traceID)
	if err != nil {
		return nil, err
	}
	_ = trace
	events, err := a.store.GetEvents(ctx, traceID)
	if err != nil {
		return nil, err
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return events, nil
	}
	filtered := make([]*ReasoningEvent, 0, len(events))
	for _, e := range events {
		if strings.EqualFold(e.EventType, eventType) {
			filtered = append(filtered, e)
		}
	}
	return filtered, nil
}
