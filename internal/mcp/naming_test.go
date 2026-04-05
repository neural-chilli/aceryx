package mcp

import "testing"

func TestToolNamePrefixing(t *testing.T) {
	got := PrefixToolName("internal_crm", "search_documents")
	if got != "mcp_internal_crm_search_documents" {
		t.Fatalf("unexpected prefixed name: %s", got)
	}
	prefix, tool, err := UnprefixToolName(got)
	if err != nil {
		t.Fatalf("UnprefixToolName: %v", err)
	}
	if prefix != "internal" || tool != "crm_search_documents" {
		t.Fatalf("unexpected unprefix output %q %q", prefix, tool)
	}
}
