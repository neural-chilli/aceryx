package agentic

import "testing"

func TestValidateToolCallAndArgs(t *testing.T) {
	manifest := NewToolManifest([]ResolvedTool{{
		ID:         "t1",
		Name:       "lookup",
		Parameters: []byte(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`),
	}})
	if _, err := ValidateToolCall("missing", manifest); err == nil {
		t.Fatalf("expected missing tool error")
	}
	if _, err := ValidateToolCall("lookup", manifest); err != nil {
		t.Fatalf("expected existing tool, got %v", err)
	}
	if err := ValidateToolArgs(`{"q":"ok"}`, manifest.tools[0].Parameters); err != nil {
		t.Fatalf("expected valid args, got %v", err)
	}
	if err := ValidateToolArgs(`{"q":1}`, manifest.tools[0].Parameters); err == nil {
		t.Fatalf("expected invalid args")
	}
}
