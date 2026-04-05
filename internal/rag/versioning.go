package rag

import (
	"context"
	"fmt"
)

func CheckEmbeddingCompatibility(ctx context.Context, kb *KnowledgeBase, store KnowledgeBaseStore) error {
	if kb == nil || store == nil {
		return nil
	}
	mismatch, err := store.HasMismatchedEmbeddingModel(ctx, kb.TenantID, kb.ID, kb.EmbeddingModel)
	if err != nil {
		return err
	}
	if mismatch {
		if err := store.SetStatus(ctx, kb.TenantID, kb.ID, "stale_embeddings"); err != nil {
			return err
		}
		kb.Status = "stale_embeddings"
		return fmt.Errorf("embedding model changed. re-index required for consistent search")
	}
	return nil
}

func IsUploadAllowed(kb *KnowledgeBase) bool {
	if kb == nil {
		return false
	}
	return kb.Status != "stale_embeddings"
}
