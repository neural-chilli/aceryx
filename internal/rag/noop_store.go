package rag

import (
	"context"
	"fmt"
)

type NoopVectorStore struct{}

func NewNoopVectorStore() *NoopVectorStore { return &NoopVectorStore{} }

func (s *NoopVectorStore) Store(_ context.Context, _, _ string, _ []StorableChunk) error {
	return fmt.Errorf("vector store unavailable")
}

func (s *NoopVectorStore) Search(_ context.Context, _ []float32, _ SearchOpts) ([]SearchResult, error) {
	return nil, fmt.Errorf("vector store unavailable")
}

func (s *NoopVectorStore) Delete(_ context.Context, _ string) error { return nil }

func (s *NoopVectorStore) DeleteAll(_ context.Context, _ string) error { return nil }

func (s *NoopVectorStore) Count(_ context.Context, _ string) (int, error) { return 0, nil }
