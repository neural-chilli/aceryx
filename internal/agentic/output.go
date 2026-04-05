package agentic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

func parseConclusion(response string, schema json.RawMessage) (json.RawMessage, *float64, error) {
	candidate := strings.TrimSpace(response)
	if candidate == "" {
		return nil, nil, fmt.Errorf("empty model response")
	}

	raw := json.RawMessage(candidate)
	if !json.Valid(raw) {
		block, ok := extractJSONBlock(candidate)
		if !ok {
			return nil, nil, fmt.Errorf("response does not contain valid json")
		}
		raw = json.RawMessage(block)
	}

	if err := validateJSONAgainstSchema(raw, schema); err != nil {
		return nil, nil, err
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, nil, fmt.Errorf("decode conclusion: %w", err)
	}
	var confidence *float64
	if v, ok := body["confidence"].(float64); ok {
		confidence = &v
	}
	return raw, confidence, nil
}

func validateJSONAgainstSchema(payload json.RawMessage, schema json.RawMessage) error {
	if len(schema) == 0 || strings.TrimSpace(string(schema)) == "" || strings.TrimSpace(string(schema)) == "null" {
		return nil
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("agentic.schema.json", bytes.NewReader(schema)); err != nil {
		return fmt.Errorf("invalid output schema: %w", err)
	}
	compiled, err := compiler.Compile("agentic.schema.json")
	if err != nil {
		return fmt.Errorf("invalid output schema: %w", err)
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return fmt.Errorf("invalid json output: %w", err)
	}
	if err := compiled.Validate(value); err != nil {
		return fmt.Errorf("output schema validation failed: %w", err)
	}
	return nil
}

func extractJSONBlock(input string) (string, bool) {
	start := strings.Index(input, "{")
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(input); i++ {
		ch := input[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				fragment := strings.TrimSpace(input[start : i+1])
				if json.Valid([]byte(fragment)) {
					return fragment, true
				}
				return "", false
			}
		}
	}
	return "", false
}
