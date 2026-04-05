package triggers

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// ChannelPipeline processes inbound data through the unified channel pipeline.
// Spec 022 provides the full implementation; this package includes a stub.
type ChannelPipeline interface {
	Process(ctx context.Context, req PipelineRequest) (PipelineResult, error)
}

type PipelineRequest struct {
	TenantID    uuid.UUID         `json:"tenant_id"`
	ChannelID   uuid.UUID         `json:"channel_id"`
	Data        json.RawMessage   `json:"data"`
	Attachments []AttachmentInput `json:"attachments"`
	Source      string            `json:"source"`
}

type PipelineResult struct {
	CaseID  uuid.UUID `json:"case_id"`
	Deduped bool      `json:"deduped"`
	EventID uuid.UUID `json:"event_id"`
}

type AttachmentInput struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}

type StubChannelPipeline struct {
	ProcessFn func(ctx context.Context, req PipelineRequest) (PipelineResult, error)
}

func NewStubChannelPipeline(fn func(ctx context.Context, req PipelineRequest) (PipelineResult, error)) *StubChannelPipeline {
	return &StubChannelPipeline{ProcessFn: fn}
}

func (s *StubChannelPipeline) Process(ctx context.Context, req PipelineRequest) (PipelineResult, error) {
	if s != nil && s.ProcessFn != nil {
		return s.ProcessFn(ctx, req)
	}
	return PipelineResult{CaseID: uuid.New(), EventID: uuid.New()}, nil
}
