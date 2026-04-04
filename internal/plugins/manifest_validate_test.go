package plugins

import (
	"strings"
	"testing"
)

func TestValidateManifest_Valid(t *testing.T) {
	m := validManifestForTest()
	errs := ValidateManifest(&m)
	if len(manifestErrors(errs)) != 0 {
		t.Fatalf("expected no hard errors, got %#v", errs)
	}
}

func TestValidateManifest_MissingRequired(t *testing.T) {
	m := validManifestForTest()
	m.ID = ""
	errs := ValidateManifest(&m)
	assertContainsError(t, errs, "manifest missing required field: id")
}

func TestValidateManifest_InvalidID(t *testing.T) {
	m := validManifestForTest()
	m.ID = "My_Plugin"
	errs := ValidateManifest(&m)
	assertContainsError(t, errs, "invalid plugin id: must match ^[a-z][a-z0-9-]{1,63}$")
}

func TestValidateManifest_ToolCapableRequirements(t *testing.T) {
	m := validManifestForTest()
	m.ToolCapable = true
	m.ToolDescription = ""
	m.ToolSafety = ""

	errs := ValidateManifest(&m)
	assertContainsError(t, errs, "tool_description required when tool_capable is true")
	assertContainsError(t, errs, "tool_safety required when tool_capable is true")
}

func TestValidateManifest_TriggerContractRules(t *testing.T) {
	trigger := validManifestForTest()
	trigger.Type = string(TriggerPlugin)
	trigger.TriggerContract = nil
	errs := ValidateManifest(&trigger)
	assertContainsError(t, errs, "trigger plugins must include trigger_contract")

	step := validManifestForTest()
	step.Type = string(StepPlugin)
	step.TriggerContract = &TriggerContract{Delivery: "at_least_once"}
	errs = ValidateManifest(&step)
	assertContainsError(t, errs, "step plugins must not include trigger_contract")
}

func TestValidateManifest_IconTooLarge(t *testing.T) {
	m := validManifestForTest()
	m.UI.IconSVG = strings.Repeat("a", 9*1024)
	errs := ValidateManifest(&m)
	assertContainsError(t, errs, "icon_svg exceeds 8KB")
}

func validManifestForTest() PluginManifest {
	return PluginManifest{
		ID:             "companies-house",
		Name:           "Companies House",
		Version:        "1.2.3",
		Type:           string(StepPlugin),
		Category:       "Financial Services",
		Tier:           "open_source",
		Maturity:       "community",
		MinHostVersion: "0.0.1",
		UI: ManifestUI{
			Description: "Look up companies",
		},
	}
}

func assertContainsError(t *testing.T, errs []ValidationError, expected string) {
	t.Helper()
	for _, err := range errs {
		if err.Severity == ValidationSeverityError && err.Message == expected {
			return
		}
	}
	t.Fatalf("expected error %q in %#v", expected, errs)
}
