package cases

import "testing"

func TestValidateCaseData_Table(t *testing.T) {
	minLen := 2
	maxLen := 4
	min := 1.0
	max := 10.0
	schema := CaseTypeSchema{Fields: map[string]SchemaField{
		"name":   {Type: "string", Required: true, MinLength: &minLen, MaxLength: &maxLen, Pattern: "^[A-Z]+$"},
		"amount": {Type: "number", Required: true, Min: &min, Max: &max},
		"status": {Type: "string", Enum: []interface{}{"open", "closed"}},
		"nested": {Type: "object", Properties: map[string]SchemaField{"code": {Type: "integer", Required: true}}},
	}}

	tests := []struct {
		name      string
		data      map[string]interface{}
		wantRules []string
	}{
		{name: "required", data: map[string]interface{}{}, wantRules: []string{"required", "required"}},
		{name: "type", data: map[string]interface{}{"name": "AB", "amount": "x", "nested": map[string]interface{}{"code": 1}}, wantRules: []string{"type"}},
		{name: "pattern", data: map[string]interface{}{"name": "ab", "amount": 2.0, "nested": map[string]interface{}{"code": 1}}, wantRules: []string{"pattern"}},
		{name: "minmax", data: map[string]interface{}{"name": "AB", "amount": 11.0, "nested": map[string]interface{}{"code": 1}}, wantRules: []string{"max"}},
		{name: "enum", data: map[string]interface{}{"name": "AB", "amount": 2.0, "status": "x", "nested": map[string]interface{}{"code": 1}}, wantRules: []string{"enum"}},
		{name: "nested required", data: map[string]interface{}{"name": "AB", "amount": 2.0, "nested": map[string]interface{}{}}, wantRules: []string{"required"}},
		{name: "unknown accepted", data: map[string]interface{}{"name": "AB", "amount": 2.0, "nested": map[string]interface{}{"code": 1}, "unknown": "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateCaseData(schema, tt.data)
			if len(tt.wantRules) == 0 {
				if len(errs) != 0 {
					t.Fatalf("expected no errors, got %+v", errs)
				}
				return
			}
			gotRules := make([]string, 0, len(errs))
			for _, e := range errs {
				gotRules = append(gotRules, e.Rule)
			}
			for _, want := range tt.wantRules {
				found := false
				for _, got := range gotRules {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected rule %s in %+v", want, errs)
				}
			}
		})
	}
}
