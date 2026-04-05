package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func ValidateOutput(output json.RawMessage, schema json.RawMessage) []ValidationError {
	if len(schema) == 0 || string(schema) == "null" {
		return nil
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("component.schema.json", bytes.NewReader(schema)); err != nil {
		return []ValidationError{{Path: "$", Message: fmt.Sprintf("invalid schema: %v", err)}}
	}
	compiled, err := compiler.Compile("component.schema.json")
	if err != nil {
		return []ValidationError{{Path: "$", Message: fmt.Sprintf("invalid schema: %v", err)}}
	}
	var value any
	if err := json.Unmarshal(output, &value); err != nil {
		return []ValidationError{{Path: "$", Message: fmt.Sprintf("invalid json: %v", err)}}
	}
	if err := compiled.Validate(value); err != nil {
		vErr, ok := err.(*jsonschema.ValidationError)
		if !ok {
			return []ValidationError{{Path: "$", Message: err.Error()}}
		}
		out := flattenValidationErrors(vErr)
		sort.Slice(out, func(i, j int) bool {
			if out[i].Path == out[j].Path {
				return out[i].Message < out[j].Message
			}
			return out[i].Path < out[j].Path
		})
		return out
	}
	return nil
}

func flattenValidationErrors(err *jsonschema.ValidationError) []ValidationError {
	if err == nil {
		return nil
	}
	if len(err.Causes) == 0 {
		return []ValidationError{{Path: normalizeInstancePath(err.InstanceLocation), Message: strings.TrimSpace(err.Message)}}
	}
	out := make([]ValidationError, 0)
	for _, cause := range err.Causes {
		out = append(out, flattenValidationErrors(cause)...)
	}
	return out
}

func normalizeInstancePath(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return "$"
	}
	if !strings.HasPrefix(p, "$") {
		return "$" + p
	}
	return p
}
