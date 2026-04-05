package agentic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

func ValidateToolCall(toolName string, manifest *ToolManifest) (*ResolvedTool, error) {
	if manifest == nil {
		return nil, fmt.Errorf("tool manifest not configured")
	}
	tool, ok := manifest.toolsByName[strings.TrimSpace(toolName)]
	if !ok || tool == nil {
		return nil, fmt.Errorf("tool not available: %s", strings.TrimSpace(toolName))
	}
	return tool, nil
}

func ValidateToolArgs(args string, paramSchema json.RawMessage) error {
	if strings.TrimSpace(args) == "" {
		args = "{}"
	}
	if !json.Valid([]byte(args)) {
		return fmt.Errorf("arguments must be valid json object")
	}
	if len(paramSchema) == 0 || strings.TrimSpace(string(paramSchema)) == "" {
		return nil
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("tool.schema.json", bytes.NewReader(paramSchema)); err != nil {
		return fmt.Errorf("invalid tool parameter schema: %w", err)
	}
	compiled, err := compiler.Compile("tool.schema.json")
	if err != nil {
		return fmt.Errorf("invalid tool parameter schema: %w", err)
	}
	var payload any
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return fmt.Errorf("arguments must be valid json object: %w", err)
	}
	if err := compiled.Validate(payload); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}
