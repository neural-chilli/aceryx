//go:build bdd

package bdd

import (
	"os"
	"strings"
	"testing"
)

func TestLoanOriginationFeatureIncludesCoreScenarios(t *testing.T) {
	raw, err := os.ReadFile("features/loan_origination.feature")
	if err != nil {
		t.Fatalf("read feature file: %v", err)
	}
	content := string(raw)
	requiredScenarios := []string{
		"Scenario: Low risk loan is auto-approved",
		"Scenario: High value loan requires underwriter review",
		"Scenario: Underwriter refers to senior",
		"Scenario: Underwriter rejects application",
		"Scenario: SLA breach escalates overdue underwriter task",
		"Scenario: Cancellation during review halts workflow progression",
	}
	for _, scenario := range requiredScenarios {
		if !strings.Contains(content, scenario) {
			t.Fatalf("missing required scenario: %s", scenario)
		}
	}
}
