package workflows

import (
	"strings"
	"testing"

	"github.com/neural-chilli/aceryx/internal/engine"
)

func TestAddMissingRequiredConfigErrors_RuleAcceptsGuardCondition(t *testing.T) {
	validation := &PublishValidationErrors{Errors: make([]PublishValidationError, 0)}
	step := engine.WorkflowStep{
		ID:        "review_decision",
		Type:      "rule",
		Condition: "case.steps.verify_extracted_details.result.action != ''",
		Outcomes: map[string][]string{
			"approved": {"insert_customer_onboarding"},
		},
	}

	addMissingRequiredConfigErrors(validation, step, map[string]any{})

	for _, err := range validation.Errors {
		if strings.Contains(err.Message, "requires expression") {
			t.Fatalf("expected guard condition to satisfy publish requirement, got error: %#v", err)
		}
	}
}

func TestAddMissingRequiredConfigErrors_RuleRequiresExpressionOrGuardCondition(t *testing.T) {
	validation := &PublishValidationErrors{Errors: make([]PublishValidationError, 0)}
	step := engine.WorkflowStep{
		ID:   "review_decision",
		Type: "rule",
	}

	addMissingRequiredConfigErrors(validation, step, map[string]any{})

	found := false
	for _, err := range validation.Errors {
		if strings.Contains(err.Message, "requires expression or guard condition") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing expression/condition error, got %#v", validation.Errors)
	}
}
