package rag

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestChunkIDDeterministic(t *testing.T) {
	docID := uuid.New()
	a := chunkID(docID, 0, "alpha")
	b := chunkID(docID, 0, "alpha")
	if a != b {
		t.Fatalf("expected deterministic chunk id")
	}
	if chunkID(docID, 1, "alpha") == a {
		t.Fatalf("expected different id for different chunk index")
	}
	if chunkID(docID, 0, "beta") == a {
		t.Fatalf("expected different id for different content")
	}
}

func TestIngestionPipelineHappyPath(t *testing.T) {
	tenantID := uuid.New()
	kbID := uuid.New()
	docID := uuid.New()

	kb := &mockKBStore{item: KnowledgeBase{ID: kbID, TenantID: tenantID, ChunkingStrategy: "fixed", ChunkSize: 16, ChunkOverlap: 2}}
	docs := &mockDocStore{doc: KnowledgeDocument{ID: docID, KnowledgeBaseID: kbID, TenantID: tenantID, ContentType: "text/plain", StorageURI: "u://1", Status: "pending"}}
	store := newMockVectorStore()
	pipeline := NewIngestionPipeline(NewLangchainLoader(), NewLangchainSplitter(), mockEmbedder{dims: 3}, store, kb, docs, &mockVaultStore{data: map[string][]byte{"u://1": []byte(strings.Repeat("policy ", 80))}})

	if err := pipeline.ProcessDocument(context.Background(), kbID, docID); err != nil {
		t.Fatalf("process document: %v", err)
	}
	if docs.doc.Status != "ready" {
		t.Fatalf("expected ready status, got %s", docs.doc.Status)
	}
	if docs.doc.ChunkCount == 0 {
		t.Fatalf("expected chunk count > 0")
	}
}

func TestIngestionPipelineExtractionFailure(t *testing.T) {
	tenantID := uuid.New()
	kbID := uuid.New()
	docID := uuid.New()

	kb := &mockKBStore{item: KnowledgeBase{ID: kbID, TenantID: tenantID, ChunkingStrategy: "fixed", ChunkSize: 16, ChunkOverlap: 2}}
	docs := &mockDocStore{doc: KnowledgeDocument{ID: docID, KnowledgeBaseID: kbID, TenantID: tenantID, ContentType: "application/pdf", StorageURI: "u://missing", Status: "pending"}}
	store := newMockVectorStore()
	pipeline := NewIngestionPipeline(NewLangchainLoader(), NewLangchainSplitter(), mockEmbedder{dims: 3}, store, kb, docs, &mockVaultStore{data: map[string][]byte{}})

	if err := pipeline.ProcessDocument(context.Background(), kbID, docID); err == nil {
		t.Fatalf("expected extraction failure")
	}
	if docs.doc.Status != "error" {
		t.Fatalf("expected error status, got %s", docs.doc.Status)
	}
}
