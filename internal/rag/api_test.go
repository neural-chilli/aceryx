package rag

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAPIReIndexCostGuard(t *testing.T) {
	tenantID := uuid.New()
	kbID := uuid.New()
	kbStore := &mockKBStore{item: KnowledgeBase{ID: kbID, TenantID: tenantID, ChunkCount: 500, ChunkSize: 400, Status: "active"}}
	docStore := &mockDocStore{}
	store := newMockVectorStore()
	api := NewAPI(kbStore, docStore, NewSearchService(store, mockEmbedder{dims: 3}, kbStore), &IngestionPipeline{}, store, ModelPricing{InputPer1MTokensUSD: 500})

	est, started, err := api.ReIndex(context.Background(), tenantID, kbID, false)
	if err != nil {
		t.Fatalf("reindex estimate: %v", err)
	}
	if started {
		t.Fatalf("expected reindex to be blocked without confirm")
	}
	if est.EstimatedCostUSD <= DefaultReIndexCostGuardUSD {
		t.Fatalf("expected estimate to exceed threshold")
	}
}
