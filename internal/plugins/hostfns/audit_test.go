package hostfns

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAuditorSummaryMode(t *testing.T) {
	a := NewAuditor("summary", 50, 10)
	start := time.Now()
	a.Record("HTTPRequest", start, nil, nil)
	a.Record("HTTPRequest", start, nil, nil)
	raw := a.JSON()
	var rows []AuditSummary
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if len(rows) != 1 || rows[0].CallCount != 2 {
		t.Fatalf("unexpected summary rows: %+v", rows)
	}
}

func TestAuditorFullMode(t *testing.T) {
	a := NewAuditor("full", 2, 1)
	start := time.Now()
	a.Record("SecretGet", start, nil, map[string]any{"key": "x"})
	a.Record("SecretGet", start, nil, map[string]any{"key": "y"})
	a.Record("SecretGet", start, nil, map[string]any{"key": "z"})
	raw := a.JSON()
	var rows []AuditEntry
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal full: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected max_entries cap, got %d", len(rows))
	}
}
