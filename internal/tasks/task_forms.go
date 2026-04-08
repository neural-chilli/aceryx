package tasks

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func extractSLAHours(metaRaw []byte) int {
	var meta map[string]any
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return 0
	}
	if v, ok := meta["sla_hours"].(float64); ok {
		return int(v)
	}
	return 0
}

func SLAStatus(now time.Time, deadline *time.Time, startedAt *time.Time, slaHours int) string {
	if deadline == nil {
		return "on_track"
	}
	if deadline.Before(now) {
		return "breached"
	}
	if slaHours > 0 {
		warningWindow := time.Duration(float64(time.Hour) * float64(slaHours) * 0.25)
		warningStart := deadline.Add(-warningWindow)
		if !now.Before(warningStart) {
			return "warning"
		}
		return "on_track"
	}
	if startedAt != nil {
		remaining := deadline.Sub(now)
		full := deadline.Sub(*startedAt)
		if full > 0 && remaining <= full/4 {
			return "warning"
		}
	}
	return "on_track"
}

func ValidateFormData(schema FormSchema, data map[string]any) []ValidationError {
	errs := make([]ValidationError, 0)
	for _, field := range flattenFormFields(schema) {
		fieldKey := formFieldDataPath(field)
		if fieldKey == "" {
			continue
		}
		v, ok := getAtPath(data, fieldKey)
		if field.Required && (!ok || isEmptyValue(v)) {
			errs = append(errs, ValidationError{Field: fieldKey, Code: "required", Message: "field is required"})
			continue
		}
		if !ok {
			continue
		}
		if s, ok := v.(string); ok {
			if field.MinLength != nil && len(s) < *field.MinLength {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "min_length", Message: fmt.Sprintf("minimum length is %d", *field.MinLength)})
			}
			if field.MaxLength != nil && len(s) > *field.MaxLength {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "max_length", Message: fmt.Sprintf("maximum length is %d", *field.MaxLength)})
			}
		}
		switch strings.ToLower(field.Type) {
		case "string":
			if _, ok := v.(string); !ok {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "type", Message: "must be a string"})
			}
		case "number":
			num, ok := toFloat(v)
			if !ok {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "type", Message: "must be a number"})
				continue
			}
			if field.Min != nil && num < *field.Min {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "min", Message: fmt.Sprintf("minimum value is %v", *field.Min)})
			}
			if field.Max != nil && num > *field.Max {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "max", Message: fmt.Sprintf("maximum value is %v", *field.Max)})
			}
		case "integer":
			num, ok := toFloat(v)
			if !ok || num != float64(int64(num)) {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "type", Message: "must be an integer"})
				continue
			}
			if field.Min != nil && num < *field.Min {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "min", Message: fmt.Sprintf("minimum value is %v", *field.Min)})
			}
			if field.Max != nil && num > *field.Max {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "max", Message: fmt.Sprintf("maximum value is %v", *field.Max)})
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "type", Message: "must be a boolean"})
			}
		case "object":
			if _, ok := v.(map[string]any); !ok {
				errs = append(errs, ValidationError{Field: fieldKey, Code: "type", Message: "must be an object"})
			}
		}
	}
	return errs
}

func ValidateActionRequirements(schema FormSchema, outcome string, data map[string]any) []ValidationError {
	if strings.TrimSpace(outcome) == "" {
		return nil
	}
	for _, action := range schema.Actions {
		if action.Value != outcome {
			continue
		}
		errs := make([]ValidationError, 0)
		for _, required := range action.Requires {
			path := strings.TrimSpace(required)
			path = strings.TrimPrefix(path, "decision.")
			if path == "" {
				continue
			}
			v, ok := getAtPath(data, path)
			if !ok || isEmptyValue(v) {
				errs = append(errs, ValidationError{Field: path, Code: "required_for_action", Message: "field is required for selected action"})
			}
		}
		return errs
	}
	return nil
}

func buildDecisionPatch(schema FormSchema, data map[string]any) map[string]any {
	out := map[string]any{}
	for _, field := range flattenFormFields(schema) {
		if !strings.HasPrefix(field.Bind, "decision.") {
			continue
		}
		writePath := strings.TrimPrefix(field.Bind, "decision.")
		if writePath == "" {
			continue
		}
		readPath := formFieldDataPath(field)
		if readPath == "" {
			readPath = writePath
		}
		val, ok := getAtPath(data, readPath)
		if !ok {
			continue
		}
		setAtPath(out, writePath, val)
	}
	return out
}

func hasFormSchemaContent(schema FormSchema) bool {
	return strings.TrimSpace(schema.Title) != "" || len(schema.Actions) > 0 || len(schema.Layout) > 0 || len(schema.Fields) > 0
}

func flattenFormFields(schema FormSchema) []FormField {
	out := make([]FormField, 0, len(schema.Fields))
	out = append(out, schema.Fields...)
	for _, section := range schema.Layout {
		out = append(out, section.Fields...)
	}
	return out
}

func formFieldDataPath(field FormField) string {
	if strings.TrimSpace(field.ID) != "" {
		return strings.TrimSpace(field.ID)
	}
	bind := strings.TrimSpace(field.Bind)
	if strings.HasPrefix(bind, "decision.") {
		return strings.TrimPrefix(bind, "decision.")
	}
	return bind
}

func getAtPath(obj map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	cur := any(obj)
	for _, part := range parts {
		next, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := next[part]
		if !ok {
			return nil, false
		}
		cur = value
	}
	return cur, true
}

func setAtPath(obj map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := obj
	for i, part := range parts {
		if i == len(parts)-1 {
			cur[part] = value
			return
		}
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	if arr, ok := v.([]any); ok {
		return len(arr) == 0
	}
	if arr, ok := v.([]string); ok {
		return len(arr) == 0
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func contains(items []string, val string) bool {
	for _, item := range items {
		if item == val {
			return true
		}
	}
	return false
}

func inboxLess(a, b InboxTask, now time.Time) bool {
	as := SLAStatus(now, a.SLADeadline, a.StartedAt, a.SLAHours)
	bs := SLAStatus(now, b.SLADeadline, b.StartedAt, b.SLAHours)
	order := map[string]int{"breached": 0, "warning": 1, "on_track": 2}
	if order[as] != order[bs] {
		return order[as] < order[bs]
	}
	if a.SLADeadline != nil && b.SLADeadline != nil && !a.SLADeadline.Equal(*b.SLADeadline) {
		return a.SLADeadline.Before(*b.SLADeadline)
	}
	if a.SLADeadline == nil && b.SLADeadline != nil {
		return false
	}
	if a.SLADeadline != nil && b.SLADeadline == nil {
		return true
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if a.StartedAt != nil && b.StartedAt != nil {
		return a.StartedAt.Before(*b.StartedAt)
	}
	if a.StartedAt == nil && b.StartedAt != nil {
		return false
	}
	if a.StartedAt != nil && b.StartedAt == nil {
		return true
	}
	return a.StepID < b.StepID
}

func configuredOutcomes(metadata map[string]any) []string {
	raw, ok := metadata["outcomes"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func configuredFormSchema(metadata map[string]any) FormSchema {
	raw, ok := metadata["form_schema"]
	if !ok {
		return FormSchema{}
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return FormSchema{}
	}
	var schema FormSchema
	if err := json.Unmarshal(buf, &schema); err != nil {
		return FormSchema{}
	}
	return schema
}

func extractAgentOriginalOutput(metadata map[string]any) map[string]any {
	reviewRaw, ok := metadata["agent_review"]
	if !ok {
		return map[string]any{}
	}
	reviewMap, ok := reviewRaw.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	orig, ok := reviewMap["original_output"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return orig
}
