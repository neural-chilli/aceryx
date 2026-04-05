package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/mcpserver"
	"github.com/neural-chilli/aceryx/internal/rag"
)

type SearchKnowledgeBaseTool struct {
	Search mcpserver.SearchService
	KBs    mcpserver.KBStore
}

func (t *SearchKnowledgeBaseTool) Name() string               { return "search_knowledge_base" }
func (t *SearchKnowledgeBaseTool) RequiredPermission() string { return "workflows:view" }
func (t *SearchKnowledgeBaseTool) Definition() mcpserver.ToolDefinition {
	return mcpserver.ToolDefinition{
		Name:        t.Name(),
		Description: "Search a knowledge base using hybrid semantic + keyword search.",
		InputSchema: json.RawMessage("{\"type\":\"object\",\"properties\":{\"knowledge_base\":{\"type\":\"string\"},\"query\":{\"type\":\"string\"},\"top_k\":{\"type\":\"integer\",\"default\":5},\"min_score\":{\"type\":\"number\",\"default\":0.7}},\"required\":[\"knowledge_base\",\"query\"]}"),
	}
}

func (t *SearchKnowledgeBaseTool) Execute(ctx context.Context, conn *mcpserver.Connection, args json.RawMessage) (any, error) {
	if t.Search == nil || t.KBs == nil {
		return nil, fmt.Errorf("knowledge search not configured")
	}
	var in struct {
		KnowledgeBase string  `json:"knowledge_base"`
		Query         string  `json:"query"`
		TopK          int     `json:"top_k"`
		MinScore      float64 `json:"min_score"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(in.KnowledgeBase) == "" || strings.TrimSpace(in.Query) == "" {
		return nil, fmt.Errorf("knowledge_base and query are required")
	}
	kbID, err := resolveKnowledgeBase(ctx, t.KBs, conn.TenantID, in.KnowledgeBase)
	if err != nil {
		return nil, err
	}
	resp, err := t.Search.Search(ctx, rag.SearchRequest{TenantID: conn.TenantID, KnowledgeBaseID: kbID, Query: in.Query, TopK: in.TopK, MinScore: in.MinScore, Mode: "hybrid"})
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(resp.Results))
	for _, r := range resp.Results {
		results = append(results, map[string]any{"content": r.Content, "score": r.Score, "source": r.Source, "page_number": r.Metadata.PageNumber})
	}
	return map[string]any{"results": results}, nil
}

func resolveKnowledgeBase(ctx context.Context, store mcpserver.KBStore, tenantID uuid.UUID, idOrName string) (uuid.UUID, error) {
	if parsed, err := uuid.Parse(strings.TrimSpace(idOrName)); err == nil {
		return parsed, nil
	}
	items, err := store.ListKnowledgeBases(ctx, tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	for _, item := range items {
		if strings.EqualFold(item.Name, strings.TrimSpace(idOrName)) {
			return item.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("knowledge base %q not found", idOrName)
}
