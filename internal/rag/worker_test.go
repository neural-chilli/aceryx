package rag

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWorkerProcessesPendingDocument(t *testing.T) {
	tenantID := uuid.New()
	kbID := uuid.New()
	docID := uuid.New()

	kb := &mockKBStore{item: KnowledgeBase{ID: kbID, TenantID: tenantID, ChunkingStrategy: "fixed", ChunkSize: 16, ChunkOverlap: 2}}
	docs := &mockDocStore{doc: KnowledgeDocument{ID: docID, KnowledgeBaseID: kbID, TenantID: tenantID, ContentType: "text/plain", StorageURI: "u://1", Status: "pending"}}
	store := newMockVectorStore()
	pipeline := NewIngestionPipeline(NewLangchainLoader(), NewLangchainSplitter(), mockEmbedder{dims: 3}, store, kb, docs, &mockVaultStore{data: map[string][]byte{"u://1": []byte(strings.Repeat("policy ", 80))}})
	worker := NewWorker(pipeline, docs, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if docs.status() == "ready" {
			worker.Stop()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	worker.Stop()
	t.Fatalf("expected worker to process pending doc")
}
