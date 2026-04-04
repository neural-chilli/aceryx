package plugins

import (
	"fmt"
	"strings"
)

const defaultRuntimeHostVersion = "0.0.1"

type ValidationSeverity string

const (
	ValidationSeverityError   ValidationSeverity = "error"
	ValidationSeverityWarning ValidationSeverity = "warning"
)

type ValidationError struct {
	Field    string             `json:"field"`
	Message  string             `json:"message"`
	Severity ValidationSeverity `json:"severity"`
}

func ValidateManifest(m *PluginManifest) []ValidationError {
	if m == nil {
		return []ValidationError{{Field: "manifest", Message: "manifest is nil", Severity: ValidationSeverityError}}
	}

	errList := make([]ValidationError, 0)

	required := map[string]string{
		"id":               m.ID,
		"name":             m.Name,
		"version":          m.Version,
		"type":             m.Type,
		"category":         m.Category,
		"tier":             m.Tier,
		"maturity":         m.Maturity,
		"min_host_version": m.MinHostVersion,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			errList = append(errList, ValidationError{Field: field, Message: fmt.Sprintf("manifest missing required field: %s", field), Severity: ValidationSeverityError})
		}
	}

	if m.ID != "" && !pluginIDRe.MatchString(m.ID) {
		errList = append(errList, ValidationError{Field: "id", Message: "invalid plugin id: must match ^[a-z][a-z0-9-]{1,63}$", Severity: ValidationSeverityError})
	}
	if m.Version != "" {
		if _, err := parseSemver(m.Version); err != nil {
			errList = append(errList, ValidationError{Field: "version", Message: fmt.Sprintf("invalid manifest version: %v", err), Severity: ValidationSeverityError})
		}
	}

	switch m.Type {
	case string(StepPlugin), string(TriggerPlugin):
	case "":
	default:
		errList = append(errList, ValidationError{Field: "type", Message: fmt.Sprintf("invalid plugin type: %s", m.Type), Severity: ValidationSeverityError})
	}

	switch m.Tier {
	case "open_source", "commercial":
	case "":
	default:
		errList = append(errList, ValidationError{Field: "tier", Message: fmt.Sprintf("invalid tier: %s", m.Tier), Severity: ValidationSeverityError})
	}

	switch m.Maturity {
	case "core", "certified", "community", "generated":
	case "":
	default:
		errList = append(errList, ValidationError{Field: "maturity", Message: fmt.Sprintf("invalid maturity: %s", m.Maturity), Severity: ValidationSeverityError})
	}

	if m.MinHostVersion != "" {
		if _, err := parseSemver(m.MinHostVersion); err != nil {
			errList = append(errList, ValidationError{Field: "min_host_version", Message: fmt.Sprintf("invalid min_host_version: %v", err), Severity: ValidationSeverityError})
		}
	}

	if m.ToolCapable {
		if strings.TrimSpace(m.ToolDescription) == "" {
			errList = append(errList, ValidationError{Field: "tool_description", Message: "tool_description required when tool_capable is true", Severity: ValidationSeverityError})
		}
		if strings.TrimSpace(m.ToolSafety) == "" {
			errList = append(errList, ValidationError{Field: "tool_safety", Message: "tool_safety required when tool_capable is true", Severity: ValidationSeverityError})
		}
	}

	if m.ToolSafety != "" {
		switch m.ToolSafety {
		case "read_only", "idempotent_write", "side_effect":
		default:
			errList = append(errList, ValidationError{Field: "tool_safety", Message: fmt.Sprintf("invalid tool_safety: %s", m.ToolSafety), Severity: ValidationSeverityError})
		}
	}

	if m.Type == string(TriggerPlugin) && m.TriggerContract == nil {
		errList = append(errList, ValidationError{Field: "trigger_contract", Message: "trigger plugins must include trigger_contract", Severity: ValidationSeverityError})
	}
	if m.Type == string(StepPlugin) && m.TriggerContract != nil {
		errList = append(errList, ValidationError{Field: "trigger_contract", Message: "step plugins must not include trigger_contract", Severity: ValidationSeverityError})
	}

	if (m.Maturity == "core" || m.Maturity == "certified") && m.Operational == nil {
		errList = append(errList, ValidationError{Field: "operational", Message: "core/certified plugins must include operational metadata", Severity: ValidationSeverityError})
	}

	if strings.TrimSpace(m.UI.IconSVG) != "" && len([]byte(strings.TrimSpace(m.UI.IconSVG))) > 8*1024 {
		errList = append(errList, ValidationError{Field: "ui.icon_svg", Message: "icon_svg exceeds 8KB", Severity: ValidationSeverityError})
	}

	if strings.TrimSpace(m.MaxHostVersion) != "" && strings.TrimSpace(m.MinHostVersion) != "" {
		if _, err := parseSemver(m.MinHostVersion); err == nil {
			if cmp, cmpErr := compareSemver(defaultRuntimeHostVersion, m.MinHostVersion); cmpErr == nil && cmp < 0 {
				errList = append(errList, ValidationError{Field: "max_host_version", Message: fmt.Sprintf("max_host_version set but current host version %s is below min_host_version %s", defaultRuntimeHostVersion, m.MinHostVersion), Severity: ValidationSeverityWarning})
			}
		}
	}

	if m.Cost == nil {
		errList = append(errList, ValidationError{Field: "cost", Message: "cost metadata missing, default level 'medium' assumed", Severity: ValidationSeverityWarning})
	}

	return errList
}

func manifestErrors(errs []ValidationError) []ValidationError {
	out := make([]ValidationError, 0)
	for _, err := range errs {
		if err.Severity == ValidationSeverityError {
			out = append(out, err)
		}
	}
	return out
}

func manifestWarnings(errs []ValidationError) []ValidationError {
	out := make([]ValidationError, 0)
	for _, err := range errs {
		if err.Severity == ValidationSeverityWarning {
			out = append(out, err)
		}
	}
	return out
}
