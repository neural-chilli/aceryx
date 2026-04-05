package agentic

import "testing"

func TestBuildPrompts(t *testing.T) {
	manifest := NewToolManifest([]ResolvedTool{{Name: "kb_search", Source: ToolSourceRAG, Description: "Search KB"}})
	sys := buildSystemPrompt(manifest, []byte(`{"type":"object"}`))
	if sys == "" {
		t.Fatalf("expected system prompt")
	}
	goal := buildGoalPrompt("Assess", []byte(`{"a":1}`), []byte(`{"type":"object"}`))
	if goal == "" {
		t.Fatalf("expected goal prompt")
	}
}
