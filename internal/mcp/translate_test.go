package mcp

import "testing"

func TestToLLMToolDefs(t *testing.T) {
	tools := []MCPTool{
		{Name: "tool_a", Description: "A", InputSchema: []byte(`{"type":"object"}`)},
		{Name: "tool_b", Description: "B", InputSchema: []byte(`{"type":"object"}`)},
	}
	defs := ToLLMToolDefs(tools, "internal_crm", []string{"tool_b"})
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool after filter, got %d", len(defs))
	}
	if defs[0].Name != "mcp_internal_crm_tool_b" {
		t.Fatalf("unexpected name: %s", defs[0].Name)
	}
	if defs[0].Description != "B" {
		t.Fatalf("unexpected description")
	}
}
