package rag

import "context"

// TextSplitter wraps chunking implementations behind an Aceryx interface.
type TextSplitter interface {
	Split(text string, opts SplitOpts) ([]Chunk, error)
}

type SplitOpts struct {
	Strategy     string
	ChunkSize    int
	ChunkOverlap int
}

type Chunk struct {
	Content    string
	TokenCount int
	Metadata   ChunkMetadata
}

type ChunkMetadata struct {
	PageNumber   int    `json:"page_number,omitempty"`
	SectionTitle string `json:"section_title,omitempty"`
	CharStart    int    `json:"char_start"`
	CharEnd      int    `json:"char_end"`
	ChunkIndex   int    `json:"chunk_index"`
	Strategy     string `json:"strategy"`
}

// DocumentLoader wraps document loading/extraction implementations.
type DocumentLoader interface {
	Load(data []byte, contentType string) (string, error)
	SupportedTypes() []string
}

// VectorStore abstracts vector storage and retrieval.
type VectorStore interface {
	Store(ctx context.Context, tenantID string, kbID string, chunks []StorableChunk) error
	Search(ctx context.Context, query []float32, opts SearchOpts) ([]SearchResult, error)
	Delete(ctx context.Context, documentID string) error
	DeleteAll(ctx context.Context, kbID string) error
	Count(ctx context.Context, kbID string) (int, error)
}

type StorableChunk struct {
	ID         string
	DocumentID string
	Content    string
	TokenCount int
	Metadata   ChunkMetadata
	Embedding  []float32
	Model      string
}

type SearchOpts struct {
	TenantID  string
	KBID      string
	TopK      int
	MinScore  float64
	Filters   map[string]any
	Mode      string
	QueryText string
}

type SearchResult struct {
	ChunkID    string        `json:"chunk_id"`
	Content    string        `json:"content"`
	Score      float64       `json:"score"`
	Metadata   ChunkMetadata `json:"metadata"`
	DocumentID string        `json:"document_id"`
	Source     string        `json:"source,omitempty"`
}

// Embedder wraps embedding generation.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
	ModelName() string
}

// FullTextSearcher is an optional capability implemented by vector stores that
// can execute a native keyword search.
type FullTextSearcher interface {
	FullTextSearch(ctx context.Context, query string, opts SearchOpts) ([]SearchResult, error)
}

// HybridSearcher is an optional capability implemented by vector stores that
// can execute a hybrid search in a store-optimized way.
type HybridSearcher interface {
	HybridSearch(ctx context.Context, query []float32, text string, opts SearchOpts) ([]SearchResult, error)
}
