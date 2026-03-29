package tenants

import "testing"

func TestResolveTerminology_DefaultsReturnedWhenUnset(t *testing.T) {
	resolved := ResolveTerminology(nil)
	if resolved["case"] != "case" || resolved["Case"] != "Case" {
		t.Fatalf("expected case defaults, got %#v", resolved)
	}
	if resolved["tasks"] != "tasks" || resolved["Inbox"] != "Inbox" {
		t.Fatalf("expected task/inbox defaults, got %#v", resolved)
	}
}

func TestResolveTerminology_OverridesApplied(t *testing.T) {
	resolved := ResolveTerminology(Terminology{
		"case":  "application",
		"Case":  "Application",
		"tasks": "actions",
		"Tasks": "Actions",
	})
	if resolved["case"] != "application" || resolved["Case"] != "Application" {
		t.Fatalf("expected case overrides, got %#v", resolved)
	}
	if resolved["tasks"] != "actions" || resolved["Tasks"] != "Actions" {
		t.Fatalf("expected task overrides, got %#v", resolved)
	}
	if resolved["inbox"] != "inbox" {
		t.Fatalf("expected inbox default fallback, got %#v", resolved["inbox"])
	}
}
