package channels

import (
	"context"

	"github.com/neural-chilli/aceryx/internal/triggers"
)

type TriggerPipelineAdapter struct {
	pipeline *Pipeline
}

func NewTriggerPipelineAdapter(pipeline *Pipeline) *TriggerPipelineAdapter {
	return &TriggerPipelineAdapter{pipeline: pipeline}
}

func (a *TriggerPipelineAdapter) Process(ctx context.Context, req triggers.PipelineRequest) (triggers.PipelineResult, error) {
	if a == nil || a.pipeline == nil {
		return triggers.PipelineResult{}, nil
	}
	result, err := a.pipeline.Process(ctx, PipelineRequest{
		TenantID:    req.TenantID,
		ChannelID:   req.ChannelID,
		Data:        req.Data,
		Attachments: toAttachmentInputs(req.Attachments),
		Source:      req.Source,
	})
	if err != nil {
		return triggers.PipelineResult{}, err
	}
	return triggers.PipelineResult{CaseID: result.CaseID, Deduped: result.Deduped, EventID: result.EventID}, nil
}

func toAttachmentInputs(in []triggers.AttachmentInput) []AttachmentInput {
	if len(in) == 0 {
		return nil
	}
	out := make([]AttachmentInput, 0, len(in))
	for _, item := range in {
		out = append(out, AttachmentInput{
			Filename:    item.Filename,
			ContentType: item.ContentType,
			Data:        item.Data,
		})
	}
	return out
}
