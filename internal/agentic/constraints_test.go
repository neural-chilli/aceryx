package agentic

import "testing"

func TestConstraintEnforcer(t *testing.T) {
	ce := NewConstraintEnforcer(ReasoningLimits{MaxIterations: 2, MaxToolCalls: 1, MaxTokens: 10})
	ce.IncrementIteration()
	if msg := ce.CheckIteration(); msg != "" {
		t.Fatalf("unexpected iteration msg: %s", msg)
	}
	ce.IncrementIteration()
	if msg := ce.CheckIteration(); msg == "" {
		t.Fatalf("expected iteration limit message")
	}
	ce.IncrementToolCalls()
	if msg := ce.CheckToolCalls(); msg == "" {
		t.Fatalf("expected tool call limit message")
	}
	if warning, hardStop := ce.CheckTokenBudget(8); warning == "" || hardStop {
		t.Fatalf("expected token warning only")
	}
	if _, hardStop := ce.CheckTokenBudget(10); !hardStop {
		t.Fatalf("expected hard stop")
	}
}
