package rag

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestEmbeddingModelMismatchSetsStale(t *testing.T) {
	store := &mockKBStore{item: KnowledgeBase{ID: uuid.New(), TenantID: uuid.New(), EmbeddingModel: "mismatch", Status: "active"}}
	kb := store.item
	if err := CheckEmbeddingCompatibility(context.Background(), &kb, store); err == nil {
		t.Fatalf("expected stale embeddings warning")
	}
	if kb.Status != "stale_embeddings" {
		t.Fatalf("expected stale_embeddings status")
	}
	if IsUploadAllowed(&kb) {
		t.Fatalf("expected uploads to be blocked")
	}
}

func TestEmbeddingModelMatchAllowsUpload(t *testing.T) {
	store := &mockKBStore{item: KnowledgeBase{ID: uuid.New(), TenantID: uuid.New(), EmbeddingModel: "text-embedding-3-small", Status: "active"}}
	kb := store.item
	if err := CheckEmbeddingCompatibility(context.Background(), &kb, store); err != nil {
		t.Fatalf("unexpected compatibility error: %v", err)
	}
	if !IsUploadAllowed(&kb) {
		t.Fatalf("expected uploads to remain allowed")
	}
}
