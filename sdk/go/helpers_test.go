package sdk

import "testing"

func TestConvenienceGetters(t *testing.T) {
	mock := NewMockContext()
	mock.SetCaseData("amount", "12.5")
	mock.SetCaseData("name", `"alice"`)
	mock.SetCaseData("enabled", "true")
	mock.SetConfig("retries", "3")
	mock.SetConfig("threshold", "4.5")
	mock.SetConfig("flag", "true")

	ctx := newContextWithBridge(mock)

	if got := ctx.CaseGetFloat("amount"); got != 12.5 {
		t.Fatalf("CaseGetFloat = %v", got)
	}
	if got := ctx.CaseGetString("name"); got != "alice" {
		t.Fatalf("CaseGetString = %q", got)
	}
	if got := ctx.CaseGetBool("enabled"); !got {
		t.Fatalf("CaseGetBool = %v", got)
	}

	if got := ctx.ConfigInt("retries"); got != 3 {
		t.Fatalf("ConfigInt = %d", got)
	}
	if got := ctx.ConfigFloat("threshold"); got != 4.5 {
		t.Fatalf("ConfigFloat = %v", got)
	}
	if got := ctx.ConfigBool("flag"); !got {
		t.Fatalf("ConfigBool = %v", got)
	}
}

func TestConvenienceGettersDefaults(t *testing.T) {
	ctx := newContextWithBridge(NewMockContext())

	if got := ctx.CaseGetFloat("missing"); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := ctx.CaseGetString("missing"); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := ctx.CaseGetBool("missing"); got {
		t.Fatalf("expected false")
	}
	if got := ctx.ConfigInt("missing"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := ctx.ConfigFloat("missing"); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := ctx.ConfigBool("missing"); got {
		t.Fatalf("expected false")
	}
}
