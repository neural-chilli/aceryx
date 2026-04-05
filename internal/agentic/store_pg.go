package agentic

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type PostgresTraceStore struct {
	db *sql.DB
}

func NewPostgresTraceStore(db *sql.DB) *PostgresTraceStore {
	return &PostgresTraceStore{db: db}
}

func (s *PostgresTraceStore) CreateTrace(ctx context.Context, trace *ReasoningTrace) error {
	if s == nil || s.db == nil || trace == nil {
		return nil
	}
	if trace.ID == uuid.Nil {
		trace.ID = uuid.New()
	}
	if trace.CreatedAt.IsZero() {
		trace.CreatedAt = time.Now().UTC()
	}
	if trace.Status == "" {
		trace.Status = "running"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO agentic_reasoning_traces (
    id, tenant_id, case_id, step_id, instance_id, model_used, goal, status,
    conclusion, total_iterations, total_tool_calls, total_tokens, total_duration_ms,
    created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9::jsonb, $10, $11, $12, NULLIF($13, 0),
    $14, $15
)
`, trace.ID, trace.TenantID, trace.CaseID, trace.StepID, trace.InstanceID, trace.ModelUsed, trace.Goal, trace.Status,
		string(trace.Conclusion), trace.TotalIterations, trace.TotalToolCalls, trace.TotalTokens, trace.TotalDurationMS,
		trace.CreatedAt, trace.CompletedAt)
	if err != nil {
		return fmt.Errorf("insert agentic trace: %w", err)
	}
	return nil
}

func (s *PostgresTraceStore) UpdateTrace(ctx context.Context, trace *ReasoningTrace) error {
	if s == nil || s.db == nil || trace == nil || trace.ID == uuid.Nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE agentic_reasoning_traces
SET status = $2,
    conclusion = $3::jsonb,
    total_iterations = $4,
    total_tool_calls = $5,
    total_tokens = $6,
    total_duration_ms = NULLIF($7, 0),
    completed_at = $8
WHERE id = $1
`, trace.ID, trace.Status, string(trace.Conclusion), trace.TotalIterations, trace.TotalToolCalls, trace.TotalTokens, trace.TotalDurationMS, trace.CompletedAt)
	if err != nil {
		return fmt.Errorf("update agentic trace: %w", err)
	}
	return nil
}

func (s *PostgresTraceStore) GetTrace(ctx context.Context, id uuid.UUID) (*ReasoningTrace, error) {
	if s == nil || s.db == nil {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, case_id, step_id, instance_id, model_used, goal, status,
       COALESCE(conclusion, '{}'::jsonb), total_iterations, total_tool_calls, total_tokens,
       COALESCE(total_duration_ms, 0), created_at, completed_at
FROM agentic_reasoning_traces
WHERE id = $1
`, id)
	return scanTrace(row)
}

func (s *PostgresTraceStore) GetTraceByCase(ctx context.Context, caseID uuid.UUID, stepID string) (*ReasoningTrace, error) {
	if s == nil || s.db == nil {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, case_id, step_id, instance_id, model_used, goal, status,
       COALESCE(conclusion, '{}'::jsonb), total_iterations, total_tool_calls, total_tokens,
       COALESCE(total_duration_ms, 0), created_at, completed_at
FROM agentic_reasoning_traces
WHERE case_id = $1 AND step_id = $2
ORDER BY created_at DESC
LIMIT 1
`, caseID, stepID)
	return scanTrace(row)
}

func scanTrace(row interface{ Scan(dest ...any) error }) (*ReasoningTrace, error) {
	var t ReasoningTrace
	if err := row.Scan(&t.ID, &t.TenantID, &t.CaseID, &t.StepID, &t.InstanceID, &t.ModelUsed, &t.Goal, &t.Status,
		&t.Conclusion, &t.TotalIterations, &t.TotalToolCalls, &t.TotalTokens, &t.TotalDurationMS, &t.CreatedAt, &t.CompletedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *PostgresTraceStore) AppendEvent(ctx context.Context, event *ReasoningEvent) error {
	if s == nil || s.db == nil || event == nil {
		return nil
	}
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO agentic_reasoning_events (
    id, trace_id, iteration, sequence, event_type, content,
    tool_id, tool_source, tool_safety, side_effect, token_count, duration_ms, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6::jsonb,
    NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10, NULLIF($11, 0), NULLIF($12, 0), $13
)
`, event.ID, event.TraceID, event.Iteration, event.Sequence, event.EventType, string(event.Content),
		event.ToolID, event.ToolSource, event.ToolSafety, event.SideEffect, event.TokenCount, event.DurationMS, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert agentic event: %w", err)
	}
	return nil
}

func (s *PostgresTraceStore) GetEvents(ctx context.Context, traceID uuid.UUID) ([]*ReasoningEvent, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, trace_id, iteration, sequence, event_type, content,
       COALESCE(tool_id, ''), COALESCE(tool_source, ''), COALESCE(tool_safety, ''), COALESCE(side_effect, false),
       COALESCE(token_count, 0), COALESCE(duration_ms, 0), created_at
FROM agentic_reasoning_events
WHERE trace_id = $1
ORDER BY iteration ASC, sequence ASC, created_at ASC
`, traceID)
	if err != nil {
		return nil, fmt.Errorf("query agentic events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]*ReasoningEvent, 0)
	for rows.Next() {
		var e ReasoningEvent
		if err := rows.Scan(&e.ID, &e.TraceID, &e.Iteration, &e.Sequence, &e.EventType, &e.Content,
			&e.ToolID, &e.ToolSource, &e.ToolSafety, &e.SideEffect, &e.TokenCount, &e.DurationMS, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agentic event: %w", err)
		}
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agentic events: %w", err)
	}
	return out, nil
}
