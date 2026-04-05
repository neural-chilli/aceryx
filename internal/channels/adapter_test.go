package channels

import (
	"encoding/json"
	"testing"
)

func TestAdapterApplyDirectConstantExpression(t *testing.T) {
	t.Parallel()

	engine := NewAdapterEngine()
	inbound := []byte(`{"amount":50000,"first":"Alice","last":"Smith"}`)
	cfg := AdapterConfig{
		Mappings: []FieldMapping{
			{Type: "direct", Source: "payload.amount", Target: "case.data.loan.amount"},
			{Type: "constant", Target: "case.data.source", Value: "webhook"},
			{Type: "expression", Target: "case.data.full_name", Expression: `payload.first + " " + payload.last`},
		},
	}

	out, err := engine.Apply(cfg, inbound)
	if err != nil {
		t.Fatalf("apply adapter: %v", err)
	}
	got := map[string]any{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if loan, ok := got["loan"].(map[string]any); !ok || loan["amount"] != float64(50000) {
		t.Fatalf("expected loan.amount=50000, got %#v", got)
	}
	if got["source"] != "webhook" {
		t.Fatalf("expected source=webhook, got %#v", got["source"])
	}
	if got["full_name"] != "Alice Smith" {
		t.Fatalf("expected full_name=Alice Smith, got %#v", got["full_name"])
	}
}
