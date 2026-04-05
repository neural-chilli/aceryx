package agentic

import (
	"encoding/json"
	"testing"
)

func TestBuildOutputPatch(t *testing.T) {
	patch, err := buildOutputPatch("case.data.assessment", json.RawMessage(`{"decision":"approve"}`))
	if err != nil {
		t.Fatalf("buildOutputPatch error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(patch, &decoded); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if _, ok := decoded["assessment"]; !ok {
		t.Fatalf("expected assessment key, got %v", decoded)
	}
}
