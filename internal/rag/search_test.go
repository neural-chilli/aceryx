package rag

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type vectorOnlyStore struct {
	inner *mockVectorStore
}

func (s *vectorOnlyStore) Store(ctx context.Context, tenantID string, kbID string, chunks []StorableChunk) error {
	return s.inner.Store(ctx, tenantID, kbID, chunks)
}
func (s *vectorOnlyStore) Search(ctx context.Context, query []float32, opts SearchOpts) ([]SearchResult, error) {
	return s.inner.Search(ctx, query, opts)
}
func (s *vectorOnlyStore) Delete(ctx context.Context, documentID string) error {
	return s.inner.Delete(ctx, documentID)
}
func (s *vectorOnlyStore) DeleteAll(ctx context.Context, kbID string) error {
	return s.inner.DeleteAll(ctx, kbID)
}
func (s *vectorOnlyStore) Count(ctx context.Context, kbID string) (int, error) {
	return s.inner.Count(ctx, kbID)
}

func TestSearchServiceHybrid(t *testing.T) {
	tenantID := uuid.New()
	kbID := uuid.New()
	store := newMockVectorStore()
	store.search = []SearchResult{{ChunkID: "c1", Content: "Section 4.2.1", Score: 0.9}}
	kbStore := &mockKBStore{item: KnowledgeBase{ID: kbID, TenantID: tenantID, Status: "active"}}
	ss := NewSearchService(store, mockEmbedder{dims: 3}, kbStore)

	resp, err := ss.Search(context.Background(), SearchRequest{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		Query:           "Section 4.2.1",
		Mode:            "hybrid",
	})
	if err != nil {
		t.Fatalf("search hybrid: %v", err)
	}
	if resp.Mode != "hybrid" {
		t.Fatalf("expected hybrid mode, got %s", resp.Mode)
	}
	if len(resp.Results) == 0 {
		t.Fatalf("expected search results")
	}
}

func TestSearchServiceFallsBackWhenFulltextUnsupported(t *testing.T) {
	tenantID := uuid.New()
	kbID := uuid.New()
	store := newMockVectorStore()
	store.search = []SearchResult{{ChunkID: "c1", Content: "policy", Score: 0.8}}
	kbStore := &mockKBStore{item: KnowledgeBase{ID: kbID, TenantID: tenantID, Status: "active"}}
	ss := NewSearchService(&vectorOnlyStore{inner: store}, mockEmbedder{dims: 3}, kbStore)

	resp, err := ss.Search(context.Background(), SearchRequest{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		Query:           "policy",
		Mode:            "fulltext",
	})
	if err != nil {
		t.Fatalf("search fallback: %v", err)
	}
	if resp.Mode != "vector" {
		t.Fatalf("expected vector fallback, got %s", resp.Mode)
	}
}
