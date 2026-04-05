package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
)

type RAGContextSource struct {
	searchService *SearchService
	kbStore       KnowledgeBaseStore
	caseData      func(ctx context.Context, tenantID, caseID uuid.UUID) (map[string]any, error)
}

func NewRAGContextSource(searchService *SearchService, kbStore KnowledgeBaseStore, caseDataResolver func(ctx context.Context, tenantID, caseID uuid.UUID) (map[string]any, error)) *RAGContextSource {
	return &RAGContextSource{searchService: searchService, kbStore: kbStore, caseData: caseDataResolver}
}

type ContextSourceConfig struct {
	KnowledgeBase string  `json:"knowledge_base"`
	QueryTemplate string  `json:"query_template"`
	TopK          int     `json:"top_k"`
	MinScore      float64 `json:"min_score"`
	Mode          string  `json:"mode"`
}

type ContextChunk struct {
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	Source     string  `json:"source"`
	PageNumber int     `json:"page_number"`
}

func (r *RAGContextSource) Resolve(ctx context.Context, tenantID, caseID uuid.UUID, config ContextSourceConfig) ([]ContextChunk, error) {
	if r == nil || r.searchService == nil || r.caseData == nil {
		return nil, fmt.Errorf("rag context source not configured")
	}
	caseData, err := r.caseData(ctx, tenantID, caseID)
	if err != nil {
		return nil, err
	}
	query := connectors.ResolveTemplateString(config.QueryTemplate, map[string]any{
		"case": caseData,
		"now":  time.Now().UTC().Format(time.RFC3339),
	})
	query = strings.TrimSpace(query)
	if query == "" {
		return []ContextChunk{}, nil
	}
	kbs, err := r.kbStore.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	var kbID uuid.UUID
	for _, kb := range kbs {
		if kb.Name == config.KnowledgeBase {
			kbID = kb.ID
			break
		}
	}
	if kbID == uuid.Nil {
		return nil, fmt.Errorf("knowledge base %q not found", config.KnowledgeBase)
	}
	resp, err := r.searchService.Search(ctx, SearchRequest{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		Query:           query,
		TopK:            config.TopK,
		MinScore:        config.MinScore,
		Mode:            config.Mode,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ContextChunk, 0, len(resp.Results))
	for _, result := range resp.Results {
		out = append(out, ContextChunk{
			Content:    result.Content,
			Score:      result.Score,
			Source:     result.Source,
			PageNumber: result.Metadata.PageNumber,
		})
	}
	return out, nil
}
