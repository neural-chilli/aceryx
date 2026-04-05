package invokers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/rag"
)

type kbStoreMock struct{}

func (kbStoreMock) Create(context.Context, rag.KnowledgeBase) (rag.KnowledgeBase, error) {
	return rag.KnowledgeBase{}, nil
}
func (kbStoreMock) Get(context.Context, uuid.UUID, uuid.UUID) (rag.KnowledgeBase, error) {
	return rag.KnowledgeBase{ID: uuid.New()}, nil
}
func (kbStoreMock) List(context.Context, uuid.UUID) ([]rag.KnowledgeBase, error) { return nil, nil }
func (kbStoreMock) Update(context.Context, rag.KnowledgeBase) (rag.KnowledgeBase, error) {
	return rag.KnowledgeBase{}, nil
}
func (kbStoreMock) Delete(context.Context, uuid.UUID, uuid.UUID) error            { return nil }
func (kbStoreMock) SetStatus(context.Context, uuid.UUID, uuid.UUID, string) error { return nil }
func (kbStoreMock) RecalculateCounts(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (kbStoreMock) HasMismatchedEmbeddingModel(context.Context, uuid.UUID, uuid.UUID, string) (bool, error) {
	return false, nil
}

type vectorStoreMock struct{}

func (vectorStoreMock) Store(context.Context, string, string, []rag.StorableChunk) error { return nil }
func (vectorStoreMock) Search(context.Context, []float32, rag.SearchOpts) ([]rag.SearchResult, error) {
	return []rag.SearchResult{{Content: "doc", Score: 0.9, Metadata: rag.ChunkMetadata{PageNumber: 2}}}, nil
}
func (vectorStoreMock) Delete(context.Context, string) error       { return nil }
func (vectorStoreMock) DeleteAll(context.Context, string) error    { return nil }
func (vectorStoreMock) Count(context.Context, string) (int, error) { return 0, nil }

type embedderMock struct{}

func (embedderMock) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2}}, nil
}
func (embedderMock) Dimensions() int   { return 2 }
func (embedderMock) ModelName() string { return "mock" }

func TestRAGInvoker_Invoke(t *testing.T) {
	search := rag.NewSearchService(vectorStoreMock{}, embedderMock{}, kbStoreMock{})
	inv := NewRAGInvoker(search, uuid.New(), uuid.New())
	out, err := inv.Invoke(context.Background(), json.RawMessage(`{"query":"policy","top_k":1}`))
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected result payload")
	}
}
