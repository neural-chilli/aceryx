package rag

import (
	"strings"
	"testing"
)

func TestSplitterDeterministic(t *testing.T) {
	s := NewLangchainSplitter()
	text := strings.Repeat("policy text ", 200)
	opts := SplitOpts{Strategy: "fixed", ChunkSize: 64, ChunkOverlap: 8}

	a, err := s.Split(text, opts)
	if err != nil {
		t.Fatalf("split #1: %v", err)
	}
	b, err := s.Split(text, opts)
	if err != nil {
		t.Fatalf("split #2: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("chunk count mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Content != b[i].Content {
			t.Fatalf("content mismatch at %d", i)
		}
		if a[i].Metadata != b[i].Metadata {
			t.Fatalf("metadata mismatch at %d", i)
		}
	}
}

func TestSplitterFixedStrategy(t *testing.T) {
	s := NewLangchainSplitter()
	words := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		words = append(words, "tok")
	}
	chunks, err := s.Split(strings.Join(words, " "), SplitOpts{Strategy: "fixed", ChunkSize: 512, ChunkOverlap: 50})
	if err != nil {
		t.Fatalf("split fixed: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.Metadata.ChunkIndex != i {
			t.Fatalf("expected chunk index %d, got %d", i, c.Metadata.ChunkIndex)
		}
		if c.Metadata.CharEnd <= c.Metadata.CharStart {
			t.Fatalf("invalid offsets for chunk %d", i)
		}
	}
}

func TestSplitterRecursivePrefersParagraphs(t *testing.T) {
	s := NewLangchainSplitter()
	text := "first paragraph with text\n\nsecond paragraph with more text"
	chunks, err := s.Split(text, SplitOpts{Strategy: "recursive", ChunkSize: 100, ChunkOverlap: 10})
	if err != nil {
		t.Fatalf("split recursive: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks for 2 paragraphs, got %d", len(chunks))
	}
}

func TestSplitterSliding(t *testing.T) {
	s := NewLangchainSplitter()
	text := strings.Repeat("x ", 60)
	chunks, err := s.Split(text, SplitOpts{Strategy: "sliding", ChunkSize: 20, ChunkOverlap: 5})
	if err != nil {
		t.Fatalf("split sliding: %v", err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}
}
