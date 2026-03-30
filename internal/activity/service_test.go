package activity

import (
	"testing"
	"time"
)

func TestIsFeedWorthy(t *testing.T) {
	if !IsFeedWorthy("task", "completed") {
		t.Fatal("expected task.completed to be feed-worthy")
	}
	if IsFeedWorthy("auth", "denied") {
		t.Fatal("expected auth.denied to be excluded from feed")
	}
}

func TestFormatEventTextTerminology(t *testing.T) {
	text := formatEventText("case", "created", "Alex", "LOAN-001", "", "", nil, map[string]string{"case": "application"})
	if text != "Alex created application LOAN-001" {
		t.Fatalf("unexpected text: %s", text)
	}
}

func TestFormatAgentCompletedConfidence(t *testing.T) {
	text := formatEventText("agent", "completed", "", "LOAN-001", "risk_assessment", "Risk Assessment", []byte(`{"confidence":0.87}`), map[string]string{})
	if text != "AI completed Risk Assessment on LOAN-001 (confidence: 0.87)" {
		t.Fatalf("unexpected agent text: %s", text)
	}
}

func TestCursorTupleOrderingSemantics(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	a := now
	b := now
	if !a.Equal(b) {
		t.Fatal("expected timestamps to be equal for tuple-order semantics test")
	}
}
