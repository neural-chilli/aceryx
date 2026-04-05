package store

import (
	"testing"

	"github.com/neural-chilli/aceryx/internal/rag"
)

func TestHybridSearchRRF(t *testing.T) {
	vector := []rag.SearchResult{
		{ChunkID: "a", Content: "A"},
		{ChunkID: "b", Content: "B"},
	}
	fts := []rag.SearchResult{
		{ChunkID: "b", Content: "B"},
		{ChunkID: "c", Content: "C"},
	}
	out := hybridSearch(vector, fts, 3)
	if len(out) != 3 {
		t.Fatalf("expected 3 merged results, got %d", len(out))
	}
	if out[0].ChunkID != "b" {
		t.Fatalf("expected b first due to appearing in both lists, got %s", out[0].ChunkID)
	}
}
