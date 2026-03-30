package docgenconn

import (
	"strings"
	"testing"
)

func TestTemplateHelpers(t *testing.T) {
	if got := formatCurrency("50000"); got != "£50000.00" {
		t.Fatalf("formatCurrency mismatch: %q", got)
	}
	if got := formatDate("2026-03-29"); got != "29 March 2026" {
		t.Fatalf("formatDate mismatch: %q", got)
	}
	if got := titleCase("loan approved"); got != "Loan Approved" {
		t.Fatalf("titleCase mismatch: %q", got)
	}
}

func TestBuildSimplePDF_ValidHeader(t *testing.T) {
	pdf := buildSimplePDF([]string{"Header", "Paragraph", "Footer"})
	if !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatalf("expected pdf header, got %q", string(pdf[:8]))
	}
	if !strings.Contains(string(pdf), "%%EOF") {
		t.Fatal("expected EOF marker")
	}
}
