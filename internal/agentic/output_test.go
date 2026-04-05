package agentic

import "testing"

func TestParseConclusion_JSONBlockExtraction(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"decision":{"type":"string"},"confidence":{"type":"number"}},"required":["decision"]}`)
	resp := "Reasoning...\n{\"decision\":\"approve\",\"confidence\":0.9}"
	out, conf, err := parseConclusion(resp, schema)
	if err != nil {
		t.Fatalf("parseConclusion error: %v", err)
	}
	if string(out) == "" {
		t.Fatalf("expected output json")
	}
	if conf == nil || *conf != 0.9 {
		t.Fatalf("expected confidence 0.9, got %#v", conf)
	}
}
