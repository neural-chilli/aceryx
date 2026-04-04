package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestLenientUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(path, []byte(`
id: slack
name: Slack
version: 1.0.0
type: step
category: communication
tier: open_source
maturity: community
min_host_version: 1.0.0
future_feature: true
`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, warnings, _, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest error: %v", err)
	}
	if m.ID != "slack" {
		t.Fatalf("unexpected id: %s", m.ID)
	}
	if len(warnings) == 0 {
		t.Fatal("expected unknown field warning")
	}
}

func TestParseManifestMissingRequiredField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(path, []byte(`
name: Slack
version: 1.0.0
type: step
category: communication
tier: open_source
maturity: community
min_host_version: 1.0.0
`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, _, _, err := ParseManifest(path)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}
