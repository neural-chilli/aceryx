package agentic

import (
	"testing"

	"github.com/neural-chilli/aceryx/internal/llm"
)

func TestSummariseOldToolResults(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "goal"},
		{Role: "tool", Content: "one"},
		{Role: "tool", Content: "two"},
		{Role: "assistant", Content: "reason"},
		{Role: "tool", Content: "three"},
	}
	out := summariseOldToolResults(msgs, 4)
	if len(out) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(out))
	}
	if out[1].Role != "user" {
		t.Fatalf("expected inserted summary user message")
	}
}
