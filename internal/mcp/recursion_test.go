package mcp

import "testing"

func TestSelfInvocationDetection(t *testing.T) {
	m := &Manager{selfURLs: []string{"https://aceryx.company.com:8081"}}
	if err := m.CheckSelfInvocation("http://localhost:8081"); err == nil {
		t.Fatalf("expected localhost recursion block")
	}
	if err := m.CheckSelfInvocation("https://example.com:8081"); err != nil {
		t.Fatalf("expected external url allowed: %v", err)
	}
}
