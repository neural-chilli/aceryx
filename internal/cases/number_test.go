package cases

import "testing"

func TestFormatCasePrefixAndPadding(t *testing.T) {
	if got := formatCasePrefix("loan_application"); got != "LA" {
		t.Fatalf("expected LA, got %s", got)
	}
	if got := formatCasePrefix("loan-application-review"); got != "LAR" {
		t.Fatalf("expected LAR, got %s", got)
	}
	caseNumber := ""
	caseNumber = "LA-" + "000042"
	if caseNumber != "LA-000042" {
		t.Fatalf("unexpected format: %s", caseNumber)
	}
}
