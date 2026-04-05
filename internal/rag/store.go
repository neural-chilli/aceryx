package rag

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrKnowledgeBaseNotFound     = errors.New("knowledge base not found")
	ErrKnowledgeDocumentNotFound = errors.New("knowledge document not found")
	ErrUnsupportedContentType    = errors.New("unsupported content type")
	ErrUploadsBlocked            = errors.New("uploads blocked for this knowledge base")
)

type KnowledgeBase struct {
	ID                uuid.UUID      `json:"id"`
	TenantID          uuid.UUID      `json:"tenant_id"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	ChunkingStrategy  string         `json:"chunking_strategy"`
	ChunkSize         int            `json:"chunk_size"`
	ChunkOverlap      int            `json:"chunk_overlap"`
	EmbeddingProvider string         `json:"embedding_provider"`
	EmbeddingModel    string         `json:"embedding_model"`
	EmbeddingDims     int            `json:"embedding_dims"`
	VectorStore       string         `json:"vector_store"`
	VectorStoreConfig map[string]any `json:"vector_store_config"`
	DocumentCount     int            `json:"document_count"`
	ChunkCount        int            `json:"chunk_count"`
	Status            string         `json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type KnowledgeDocument struct {
	ID              uuid.UUID `json:"id"`
	KnowledgeBaseID uuid.UUID `json:"knowledge_base_id"`
	VaultDocumentID uuid.UUID `json:"vault_document_id"`
	TenantID        uuid.UUID `json:"tenant_id,omitempty"`
	Filename        string    `json:"filename"`
	ContentType     string    `json:"content_type"`
	FileSize        int64     `json:"file_size"`
	Status          string    `json:"status"`
	ChunkCount      int       `json:"chunk_count"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	ProcessingMS    int       `json:"processing_ms,omitempty"`
	StorageURI      string    `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type KnowledgeBaseStore interface {
	List(ctx context.Context, tenantID uuid.UUID) ([]KnowledgeBase, error)
	Create(ctx context.Context, kb KnowledgeBase) (KnowledgeBase, error)
	Get(ctx context.Context, tenantID, kbID uuid.UUID) (KnowledgeBase, error)
	Update(ctx context.Context, kb KnowledgeBase) (KnowledgeBase, error)
	Delete(ctx context.Context, tenantID, kbID uuid.UUID) error
	SetStatus(ctx context.Context, tenantID, kbID uuid.UUID, status string) error
	RecalculateCounts(ctx context.Context, tenantID, kbID uuid.UUID) error
	HasMismatchedEmbeddingModel(ctx context.Context, tenantID, kbID uuid.UUID, model string) (bool, error)
}

type DocumentStore interface {
	List(ctx context.Context, tenantID, kbID uuid.UUID) ([]KnowledgeDocument, error)
	Create(ctx context.Context, doc KnowledgeDocument) (KnowledgeDocument, error)
	Get(ctx context.Context, tenantID, kbID, docID uuid.UUID) (KnowledgeDocument, error)
	SetStatus(ctx context.Context, tenantID, kbID, docID uuid.UUID, status string, errMsg string) error
	SetReady(ctx context.Context, tenantID, kbID, docID uuid.UUID, chunkCount int, processingMS int) error
	Delete(ctx context.Context, tenantID, kbID, docID uuid.UUID) error
	ListByKB(ctx context.Context, tenantID, kbID uuid.UUID) ([]KnowledgeDocument, error)
	ClaimPending(ctx context.Context) (KnowledgeDocument, bool, error)
	ResetToPendingByKB(ctx context.Context, tenantID, kbID uuid.UUID) error
}

type ModelPricing struct {
	InputPer1MTokensUSD float64
}

type CostEstimate struct {
	TotalChunks      int     `json:"total_chunks"`
	EstimatedTokens  int64   `json:"estimated_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}
