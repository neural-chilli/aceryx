package rag

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestRAGContextSourceResolve(t *testing.T) {
	tenantID := uuid.New()
	caseID := uuid.New()
	kbID := uuid.New()
	store := newMockVectorStore()
	store.search = []SearchResult{{ChunkID: "c1", Content: "Policy chunk", Score: 0.88, Metadata: ChunkMetadata{PageNumber: 2}, Source: "policy.pdf"}}
	kbStore := &mockKBStore{item: KnowledgeBase{ID: kbID, TenantID: tenantID, Name: "lending_policies", Status: "active"}}
	ss := NewSearchService(store, mockEmbedder{dims: 3}, kbStore)

	src := NewRAGContextSource(ss, kbStore, func(_ context.Context, _, _ uuid.UUID) (map[string]any, error) {
		return map[string]any{"data": map[string]any{"loan": map[string]any{"purpose": "car"}}}, nil
	})
	chunks, err := src.Resolve(context.Background(), tenantID, caseID, ContextSourceConfig{
		KnowledgeBase: "lending_policies",
		QueryTemplate: "policy for {{case.data.loan.purpose}} loans",
		TopK:          5,
		MinScore:      0.7,
		Mode:          "hybrid",
	})
	if err != nil {
		t.Fatalf("resolve rag context: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected one context chunk, got %d", len(chunks))
	}
	if chunks[0].PageNumber != 2 {
		t.Fatalf("expected page number 2, got %d", chunks[0].PageNumber)
	}
}
