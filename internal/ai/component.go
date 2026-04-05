package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	TierOpenSource = "open_source"
	TierCommercial = "commercial"
)

type AIComponentDef struct {
	ID             string            `json:"id" yaml:"id"`
	DisplayLabel   string            `json:"display_label" yaml:"display_label"`
	Category       string            `json:"category" yaml:"category"`
	Description    string            `json:"description" yaml:"description"`
	Icon           string            `json:"icon" yaml:"icon"`
	Tier           string            `json:"tier" yaml:"tier"`
	InputSchema    json.RawMessage   `json:"input_schema" yaml:"input_schema"`
	OutputSchema   json.RawMessage   `json:"output_schema" yaml:"output_schema"`
	SystemPrompt   string            `json:"system_prompt" yaml:"system_prompt"`
	UserPromptTmpl string            `json:"user_prompt_template" yaml:"user_prompt_template"`
	ModelHints     ModelHints        `json:"model_hints" yaml:"model_hints"`
	ConfigFields   []ConfigField     `json:"config_fields" yaml:"config_fields"`
	Confidence     *ConfidenceConfig `json:"confidence,omitempty" yaml:"confidence,omitempty"`
}

type ModelHints struct {
	RequiresVision bool   `json:"requires_vision" yaml:"requires_vision"`
	PreferredSize  string `json:"preferred_size" yaml:"preferred_size"`
	MaxTokens      int    `json:"max_tokens" yaml:"max_tokens"`
}

type ConfidenceConfig struct {
	FieldPath       string  `json:"field_path" yaml:"field_path"`
	AutoAcceptAbove float64 `json:"auto_accept_above" yaml:"auto_accept_above"`
	EscalateBelow   float64 `json:"escalate_below" yaml:"escalate_below"`
}

type ConfigField struct {
	Name     string   `json:"name" yaml:"name"`
	Type     string   `json:"type" yaml:"type"`
	Label    string   `json:"label" yaml:"label"`
	Required bool     `json:"required" yaml:"required"`
	Default  any      `json:"default,omitempty" yaml:"default,omitempty"`
	Options  []string `json:"options,omitempty" yaml:"options,omitempty"`
}

type rawComponentDef struct {
	ID             string            `yaml:"id"`
	DisplayLabel   string            `yaml:"display_label"`
	Category       string            `yaml:"category"`
	Description    string            `yaml:"description"`
	Icon           string            `yaml:"icon"`
	Tier           string            `yaml:"tier"`
	InputSchema    any               `yaml:"input_schema"`
	OutputSchema   any               `yaml:"output_schema"`
	SystemPrompt   string            `yaml:"system_prompt"`
	UserPromptTmpl string            `yaml:"user_prompt_template"`
	ModelHints     ModelHints        `yaml:"model_hints"`
	ConfigFields   []ConfigField     `yaml:"config_fields"`
	Confidence     *ConfidenceConfig `yaml:"confidence"`
}

func ParseComponentYAML(raw []byte) (*AIComponentDef, error) {
	var parsed rawComponentDef
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	inputSchema, err := json.Marshal(parsed.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal input_schema: %w", err)
	}
	outputSchema, err := json.Marshal(parsed.OutputSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal output_schema: %w", err)
	}
	def := &AIComponentDef{
		ID:             strings.TrimSpace(parsed.ID),
		DisplayLabel:   strings.TrimSpace(parsed.DisplayLabel),
		Category:       strings.TrimSpace(parsed.Category),
		Description:    strings.TrimSpace(parsed.Description),
		Icon:           strings.TrimSpace(parsed.Icon),
		Tier:           strings.TrimSpace(parsed.Tier),
		InputSchema:    json.RawMessage(inputSchema),
		OutputSchema:   json.RawMessage(outputSchema),
		SystemPrompt:   strings.TrimSpace(parsed.SystemPrompt),
		UserPromptTmpl: strings.TrimSpace(parsed.UserPromptTmpl),
		ModelHints:     parsed.ModelHints,
		ConfigFields:   parsed.ConfigFields,
		Confidence:     parsed.Confidence,
	}
	if err := ValidateComponentDef(def); err != nil {
		return nil, err
	}
	return def, nil
}

func ValidateComponentDef(def *AIComponentDef) error {
	if def == nil {
		return fmt.Errorf("component definition is nil")
	}
	if def.ID == "" {
		return fmt.Errorf("id is required")
	}
	if def.DisplayLabel == "" {
		return fmt.Errorf("display_label is required")
	}
	if def.Category == "" {
		return fmt.Errorf("category is required")
	}
	if def.SystemPrompt == "" {
		return fmt.Errorf("system_prompt is required")
	}
	if def.UserPromptTmpl == "" {
		return fmt.Errorf("user_prompt_template is required")
	}
	if len(def.InputSchema) == 0 || string(def.InputSchema) == "null" {
		return fmt.Errorf("input_schema is required")
	}
	if len(def.OutputSchema) == 0 || string(def.OutputSchema) == "null" {
		return fmt.Errorf("output_schema is required")
	}
	if def.Tier == "" {
		def.Tier = TierCommercial
	}
	if def.Tier != TierCommercial && def.Tier != TierOpenSource {
		return fmt.Errorf("tier must be %q or %q", TierOpenSource, TierCommercial)
	}
	size := strings.ToLower(strings.TrimSpace(def.ModelHints.PreferredSize))
	if size != "" && size != "small" && size != "medium" && size != "large" {
		return fmt.Errorf("model_hints.preferred_size must be small, medium, or large")
	}
	def.ModelHints.PreferredSize = size
	if def.ModelHints.MaxTokens <= 0 {
		def.ModelHints.MaxTokens = 512
	}
	if def.Confidence != nil {
		if strings.TrimSpace(def.Confidence.FieldPath) == "" {
			return fmt.Errorf("confidence.field_path is required when confidence is configured")
		}
		if def.Confidence.EscalateBelow < 0 || def.Confidence.AutoAcceptAbove > 1 || def.Confidence.EscalateBelow > 1 || def.Confidence.AutoAcceptAbove < 0 {
			return fmt.Errorf("confidence thresholds must be between 0 and 1")
		}
		if def.Confidence.EscalateBelow > def.Confidence.AutoAcceptAbove {
			return fmt.Errorf("confidence.escalate_below cannot exceed confidence.auto_accept_above")
		}
	}
	for i := range def.ConfigFields {
		cf := &def.ConfigFields[i]
		cf.Name = strings.TrimSpace(cf.Name)
		cf.Type = strings.TrimSpace(strings.ToLower(cf.Type))
		cf.Label = strings.TrimSpace(cf.Label)
		if cf.Name == "" {
			return fmt.Errorf("config_fields[%d].name is required", i)
		}
		if cf.Label == "" {
			return fmt.Errorf("config_fields[%d].label is required", i)
		}
		switch cf.Type {
		case "string", "number", "select", "boolean":
		default:
			return fmt.Errorf("config_fields[%d].type must be string, number, select, or boolean", i)
		}
		if cf.Type == "select" && len(cf.Options) == 0 {
			return fmt.Errorf("config_fields[%d].options is required for select type", i)
		}
	}
	return nil
}
