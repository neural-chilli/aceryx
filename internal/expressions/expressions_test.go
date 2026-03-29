package expressions

import (
	"strings"
	"testing"
)

func TestEvaluator_Table(t *testing.T) {
	ev := NewEvaluator()

	tests := []struct {
		name      string
		expr      string
		ctx       map[string]interface{}
		wantBool  *bool
		wantError bool
	}{
		{
			name:     "simple comparison",
			expr:     "case.amount > 100",
			ctx:      map[string]interface{}{"case": map[string]interface{}{"amount": 150}},
			wantBool: boolPtr(true),
		},
		{
			name:     "boolean logic",
			expr:     "a && b || c",
			ctx:      map[string]interface{}{"a": true, "b": false, "c": true},
			wantBool: boolPtr(true),
		},
		{
			name:     "nested access",
			expr:     "case.steps.risk.result.score > 0.7",
			ctx:      map[string]interface{}{"case": map[string]interface{}{"steps": map[string]interface{}{"risk": map[string]interface{}{"result": map[string]interface{}{"score": 0.8}}}}},
			wantBool: boolPtr(true),
		},
		{
			name:     "arithmetic",
			expr:     "case.amount * 0.05 === 50",
			ctx:      map[string]interface{}{"case": map[string]interface{}{"amount": 1000}},
			wantBool: boolPtr(true),
		},
		{
			name:     "missing field undefined not error",
			expr:     "case.missing === undefined",
			ctx:      map[string]interface{}{"case": map[string]interface{}{"amount": 1000}},
			wantBool: boolPtr(true),
		},
		{
			name:      "invalid syntax",
			expr:      "case.amount >",
			ctx:       map[string]interface{}{"case": map[string]interface{}{"amount": 1000}},
			wantError: true,
		},
		{
			name:      "timeout",
			expr:      "(function(){ while(true){} return true; })()",
			ctx:       map[string]interface{}{},
			wantError: true,
		},
		{
			name:      "size limit",
			expr:      strings.Repeat("a", maxExpressionSize+1),
			ctx:       map[string]interface{}{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ev.EvaluateBool(tt.expr, tt.ctx)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error for %q", tt.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.name, err)
			}
			if tt.wantBool != nil && got != *tt.wantBool {
				t.Fatalf("expected %v, got %v", *tt.wantBool, got)
			}
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}
