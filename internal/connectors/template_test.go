package connectors

import "testing"

func TestResolveTemplateStringAndFunctions(t *testing.T) {
	ctx := map[string]any{
		"case": map[string]any{
			"data":  map[string]any{"applicant": map[string]any{"company_name": "Acme Ltd"}},
			"steps": map[string]any{"review": map[string]any{"result": map[string]any{"outcome": "approved"}}},
		},
		"tenant": map[string]any{"branding": map[string]any{"company_name": "Acme"}},
		"__secret_resolver": func(key string) string {
			if key == "api_key" {
				return "secret-123"
			}
			return ""
		},
	}

	got := ResolveTemplateString("{{case.data.applicant.company_name}}", ctx)
	if got != "Acme Ltd" {
		t.Fatalf("expected Acme Ltd, got %q", got)
	}
	got = ResolveTemplateString("{{case.steps.review.result.outcome}}", ctx)
	if got != "approved" {
		t.Fatalf("expected approved, got %q", got)
	}
	got = ResolveTemplateString("{{case.data.missing}}", ctx)
	if got != "" {
		t.Fatalf("expected empty for missing path, got %q", got)
	}
	got = ResolveTemplateString("{{secrets.api_key}}", ctx)
	if got != "secret-123" {
		t.Fatalf("expected secret value, got %q", got)
	}
	got = ResolveTemplateString("{{tenant.branding.company_name}}", ctx)
	if got != "Acme" {
		t.Fatalf("expected tenant branding company name, got %q", got)
	}
}
