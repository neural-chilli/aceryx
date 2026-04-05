package rag

import (
	"context"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/llm"
)

type AdapterEmbedder struct {
	llm      *llm.AdapterManager
	tenantID uuid.UUID
	model    string
	dims     int
}

func NewAdapterEmbedder(llmManager *llm.AdapterManager, tenantID uuid.UUID, model string, dims int) *AdapterEmbedder {
	return &AdapterEmbedder{llm: llmManager, tenantID: tenantID, model: model, dims: dims}
}

func (e *AdapterEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e == nil || e.llm == nil {
		return nil, llm.ErrProviderUnavailable
	}
	return e.llm.Embed(ctx, e.tenantID, texts, e.model)
}

func (e *AdapterEmbedder) Dimensions() int {
	if e == nil {
		return 0
	}
	return e.dims
}

func (e *AdapterEmbedder) ModelName() string {
	if e == nil {
		return ""
	}
	return e.model
}
