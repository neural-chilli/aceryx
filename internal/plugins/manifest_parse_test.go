package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifest_LenientUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	raw := `
id: slack
name: Slack
version: 1.0.0
type: step
category: communication
tier: open_source
maturity: community
min_host_version: 1.0.0
future_feature: true
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	m, warnings, _, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest error: %v", err)
	}
	if m.ID != "slack" {
		t.Fatalf("unexpected id: %s", m.ID)
	}
	if !containsWarning(warnings, "manifest slack: unknown field 'future_feature' ignored") {
		t.Fatalf("expected unknown field warning, got %#v", warnings)
	}
}

func TestParseManifest_KnownFieldsAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	raw := `
id: companies-house
name: Companies House
version: 1.0.0
type: step
category: Financial Services
tier: open_source
maturity: community
min_host_version: 1.0.0
ui:
  description: Lookup details
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, _, _, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest error: %v", err)
	}
	if m.Audit == nil || m.Audit.HostCalls.Mode != "summary" || m.Audit.HostCalls.MaxEntries != 50 || m.Audit.HostCalls.SampleRate != 10 {
		t.Fatalf("expected default audit config, got %#v", m.Audit)
	}
	if m.Cost == nil || m.Cost.Level != "medium" {
		t.Fatalf("expected default cost level medium, got %#v", m.Cost)
	}
}

func TestParseManifest_RejectsOversizedIcon(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	icon := strings.Repeat("x", 9*1024)
	raw := `
id: companies-house
name: Companies House
version: 1.0.0
type: step
category: Financial Services
tier: open_source
maturity: community
min_host_version: 1.0.0
ui:
  icon_svg: "` + icon + `"
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, _, _, err := ParseManifest(path)
	if err == nil || !strings.Contains(err.Error(), "icon_svg exceeds 8KB") {
		t.Fatalf("expected icon size error, got %v", err)
	}
}

func containsWarning(warnings []string, expected string) bool {
	for _, warning := range warnings {
		if warning == expected {
			return true
		}
	}
	return false
}
