package workflows

import (
	"strings"
	"testing"

	"github.com/neural-chilli/aceryx/internal/engine"
)

func TestAddMissingRequiredConfigErrors_RuleDoesNotRequireExpression(t *testing.T) {
	validation := &PublishValidationErrors{Errors: make([]PublishValidationError, 0)}
	step := engine.WorkflowStep{
		ID:   "review_decision",
		Type: "rule",
		Outcomes: map[string][]string{
			"approved": {"insert_customer_onboarding"},
		},
	}

	addMissingRequiredConfigErrors(validation, step, map[string]any{})

	for _, err := range validation.Errors {
		if strings.Contains(err.Message, "rule requires expression") {
			t.Fatalf("unexpected expression requirement for rule step: %#v", err)
		}
	}
}

func TestAddMissingRequiredConfigErrors_RuleRequiresOutcomes(t *testing.T) {
	validation := &PublishValidationErrors{Errors: make([]PublishValidationError, 0)}
	step := engine.WorkflowStep{
		ID:   "review_decision",
		Type: "rule",
	}

	addMissingRequiredConfigErrors(validation, step, map[string]any{})

	found := false
	for _, err := range validation.Errors {
		if strings.Contains(err.Message, "rule requires outcomes") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing outcomes error, got %#v", validation.Errors)
	}
}
