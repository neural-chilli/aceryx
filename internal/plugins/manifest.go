package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var pluginIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

var knownManifestFields = map[string]struct{}{
	"id":               {},
	"name":             {},
	"version":          {},
	"type":             {},
	"category":         {},
	"tier":             {},
	"maturity":         {},
	"min_host_version": {},
	"max_host_version": {},
	"tool_capable":     {},
	"tool_description": {},
	"tool_safety":      {},
	"ui":               {},
	"host_functions":   {},
	"operational":      {},
	"cost":             {},
	"audit":            {},
	"trigger_contract": {},
	"trigger_config":   {},
}

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
	for key := range meta {
		if _, ok := knownManifestFields[key]; !ok {
			warnings = append(warnings, fmt.Sprintf("unknown manifest field ignored: %s", key))
		}
	}

	if err := validateManifest(out); err != nil {
		return PluginManifest{}, warnings, manifestHash, err
	}
	if out.Audit.HostCalls.Mode == "" {
		out.Audit.HostCalls.Mode = "summary"
	}
	if out.Audit.HostCalls.MaxEntries <= 0 {
		out.Audit.HostCalls.MaxEntries = 50
	}
	if out.Audit.HostCalls.SampleRate <= 0 {
		out.Audit.HostCalls.SampleRate = 10
	}
	return out, warnings, manifestHash, nil
}

func validateManifest(m PluginManifest) error {
	required := map[string]string{
		"id":               m.ID,
		"name":             m.Name,
		"version":          m.Version,
		"type":             string(m.Type),
		"category":         m.Category,
		"tier":             m.Tier,
		"maturity":         m.Maturity,
		"min_host_version": m.MinHostVersion,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("manifest missing required field: %s", field)
		}
	}
	if !pluginIDRe.MatchString(m.ID) {
		return fmt.Errorf("invalid plugin id: %s", m.ID)
	}
	if _, err := parseSemver(m.Version); err != nil {
		return fmt.Errorf("invalid manifest version: %w", err)
	}
	switch m.Type {
	case StepPlugin, TriggerPlugin:
	default:
		return fmt.Errorf("invalid plugin type: %s", m.Type)
	}
	switch m.Tier {
	case "open_source", "commercial":
	default:
		return fmt.Errorf("invalid tier: %s", m.Tier)
	}
	switch m.Maturity {
	case "core", "certified", "community", "generated":
	default:
		return fmt.Errorf("invalid maturity: %s", m.Maturity)
	}
	return nil
}
