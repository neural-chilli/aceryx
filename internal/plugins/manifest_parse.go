package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

func ParseManifest(path string) (PluginManifest, []string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return PluginManifest{}, nil, "", fmt.Errorf("read manifest: %w", err)
	}
	hash := sha256.Sum256(raw)
	manifestHash := hex.EncodeToString(hash[:])

	meta := map[string]any{}
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return PluginManifest{}, nil, "", fmt.Errorf("parse manifest yaml: %w", err)
	}

	out := PluginManifest{}
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return PluginManifest{}, nil, "", fmt.Errorf("decode manifest yaml: %w", err)
	}
	out.Metadata = meta

	warnings := make([]string, 0)
	unknown := make([]string, 0)
	for key := range meta {
		if _, ok := knownManifestFields[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		warnings = append(warnings, fmt.Sprintf("manifest %s: unknown field '%s' ignored", out.ID, field))
	}

	allValidation := ValidateManifest(&out)
	for _, w := range manifestWarnings(allValidation) {
		warnings = append(warnings, w.Message)
	}
	if errs := manifestErrors(allValidation); len(errs) > 0 {
		return PluginManifest{}, warnings, manifestHash, fmt.Errorf("%s", errs[0].Message)
	}

	applyManifestDefaults(&out)
	return out, warnings, manifestHash, nil
}

func applyManifestDefaults(m *PluginManifest) {
	if m == nil {
		return
	}
	if m.Audit == nil {
		m.Audit = &AuditConfig{}
	}
	if m.Audit.HostCalls.Mode == "" {
		m.Audit.HostCalls.Mode = "summary"
	}
	if m.Audit.HostCalls.MaxEntries <= 0 {
		m.Audit.HostCalls.MaxEntries = 50
	}
	if m.Audit.HostCalls.SampleRate <= 0 {
		m.Audit.HostCalls.SampleRate = 10
	}
	if m.Cost == nil {
		m.Cost = &CostMeta{Level: "medium"}
	}
	if m.Cost.Level == "" {
		m.Cost.Level = "medium"
	}
}
