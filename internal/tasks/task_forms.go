package tasks

import (
	"encoding/json"
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
	for _, field := range schema.Fields {
		v, ok := data[field.ID]
		if field.Required && !ok {
			errs = append(errs, ValidationError{Field: field.ID, Code: "required", Message: "field is required"})
			continue
		}
		if !ok {
			continue
		}
		switch strings.ToLower(field.Type) {
		case "string":
			if _, ok := v.(string); !ok {
				errs = append(errs, ValidationError{Field: field.ID, Code: "type", Message: "must be a string"})
			}
		case "number":
			switch v.(type) {
			case float64, float32, int, int64, int32, uint, uint64, json.Number:
			default:
				errs = append(errs, ValidationError{Field: field.ID, Code: "type", Message: "must be a number"})
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				errs = append(errs, ValidationError{Field: field.ID, Code: "type", Message: "must be a boolean"})
			}
		case "object":
			if _, ok := v.(map[string]any); !ok {
				errs = append(errs, ValidationError{Field: field.ID, Code: "type", Message: "must be an object"})
			}
		}
	}
	return errs
}

func buildDecisionPatch(schema FormSchema, data map[string]any) map[string]any {
	out := map[string]any{}
	for _, field := range schema.Fields {
		if !strings.HasPrefix(field.Bind, "decision.") {
			continue
		}
		val, ok := data[field.ID]
		if !ok {
			continue
		}
		key := strings.TrimPrefix(field.Bind, "decision.")
		out[key] = val
	}
	return out
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
