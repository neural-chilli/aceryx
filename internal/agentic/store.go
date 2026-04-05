package agentic

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type TraceStore interface {
	CreateTrace(ctx context.Context, trace *ReasoningTrace) error
	UpdateTrace(ctx context.Context, trace *ReasoningTrace) error
	GetTrace(ctx context.Context, id uuid.UUID) (*ReasoningTrace, error)
	GetTraceByCase(ctx context.Context, caseID uuid.UUID, stepID string) (*ReasoningTrace, error)

	AppendEvent(ctx context.Context, event *ReasoningEvent) error
	GetEvents(ctx context.Context, traceID uuid.UUID) ([]*ReasoningEvent, error)
}

type ReasoningTrace struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	CaseID          uuid.UUID
	StepID          string
	InstanceID      uuid.UUID
	ModelUsed       string
	Goal            string
	Status          string
	Conclusion      json.RawMessage
	TotalIterations int
	TotalToolCalls  int
	TotalTokens     int
	TotalDurationMS int
	CreatedAt       time.Time
	CompletedAt     *time.Time
}

type ReasoningEvent struct {
	ID         uuid.UUID
	TraceID    uuid.UUID
	Iteration  int
	Sequence   int
	EventType  string
	Content    json.RawMessage
	ToolID     string
	ToolSource string
	ToolSafety string
	SideEffect bool
	TokenCount int
	DurationMS int
	CreatedAt  time.Time
}
