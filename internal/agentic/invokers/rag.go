package invokers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/rag"
)

type RAGInvoker struct {
	searchService *rag.SearchService
	tenantID      uuid.UUID
	kbID          uuid.UUID
}

func NewRAGInvoker(searchService *rag.SearchService, tenantID, kbID uuid.UUID) *RAGInvoker {
	return &RAGInvoker{
		searchService: searchService,
		tenantID:      tenantID,
		kbID:          kbID,
	}
}

func (ri *RAGInvoker) Invoke(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if ri == nil || ri.searchService == nil {
		return nil, fmt.Errorf("rag invoker not configured")
	}
	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("decode rag args: %w", err)
	}
	resp, err := ri.searchService.Search(ctx, rag.SearchRequest{
		TenantID:        ri.tenantID,
		KnowledgeBaseID: ri.kbID,
		Query:           req.Query,
		TopK:            req.TopK,
	})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(resp.Results))
	for _, r := range resp.Results {
		out = append(out, map[string]any{
			"content":     r.Content,
			"score":       r.Score,
			"source":      r.Source,
			"page_number": r.Metadata.PageNumber,
		})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal rag result: %w", err)
	}
	return raw, nil
}
