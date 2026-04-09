package workflows

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/expressions"
)

const (
	agentLowConfidenceEscalate = "escalate_to_human"
)

func validateWorkflowAST(ast json.RawMessage) error {
	var workflow engine.WorkflowAST
	if err := json.Unmarshal(ast, &workflow); err != nil {
		return fmt.Errorf("invalid workflow ast: decode workflow ast: %w", err)
	}
	if err := engine.ValidateAST(workflow); err != nil {
		return fmt.Errorf("invalid workflow ast: %w", err)
	}
	stepIDs := make(map[string]struct{}, len(workflow.Steps))
	for _, step := range workflow.Steps {
		stepIDs[step.ID] = struct{}{}
	}
	exprEval := expressions.NewEvaluator()
	for _, step := range workflow.Steps {
		if err := validateOutcomeTargets(step, stepIDs); err != nil {
			return fmt.Errorf("invalid workflow ast: %w", err)
		}
		if err := validateStepRequiredConfig(step); err != nil {
			return fmt.Errorf("invalid workflow ast: %w", err)
		}
		if err := validateStepExpressions(exprEval, step); err != nil {
			return fmt.Errorf("invalid workflow ast: %w", err)
		}
		switch strings.TrimSpace(step.Type) {
		case "agent":
			if err := validateAgentStepConfig(step.ID, step.Config); err != nil {
				return fmt.Errorf("invalid workflow ast: %w", err)
			}
		case "ai_component":
			if err := validateAIComponentStepConfig(step.ID, step.Config); err != nil {
				return fmt.Errorf("invalid workflow ast: %w", err)
			}
		case "extraction":
			if err := validateExtractionStepConfig(step.ID, step.Config); err != nil {
				return fmt.Errorf("invalid workflow ast: %w", err)
			}
		}
	}
	return nil
}

func validateOutcomeTargets(step engine.WorkflowStep, ids map[string]struct{}) error {
	for outcome, targets := range step.Outcomes {
		for _, target := range targets {
			if _, ok := ids[target]; !ok {
				return fmt.Errorf("step %q outcome %q targets unknown step %q", step.ID, outcome, target)
			}
		}
	}
	return nil
}

func validateStepRequiredConfig(step engine.WorkflowStep) error {
	cfg, err := decodeStepConfig(step)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(step.Type) {
	case "human_task":
		if !hasStringValue(cfg, "assign_to_role") && !hasStringValue(cfg, "assign_to_user") {
			return fmt.Errorf("step %q human_task requires assign_to_role or assign_to_user", step.ID)
		}
		if !hasKey(cfg, "form") && !hasKey(cfg, "form_schema") {
			return fmt.Errorf("step %q human_task requires form_schema (or legacy form)", step.ID)
		}
	case "agent":
		if !hasStringValue(cfg, "prompt_template") {
			return fmt.Errorf("step %q agent requires prompt_template", step.ID)
		}
	case "integration":
		if !hasStringValue(cfg, "connector") || !hasStringValue(cfg, "action") {
			return fmt.Errorf("step %q integration requires connector and action", step.ID)
		}
	case "rule":
		hasLegacyRuleOutcomes := false
		if rawOutcomes, ok := cfg["outcomes"]; ok {
			switch typed := rawOutcomes.(type) {
			case []any:
				hasLegacyRuleOutcomes = len(typed) > 0
			case map[string]any:
				hasLegacyRuleOutcomes = len(typed) > 0
			}
		}
		if len(step.Outcomes) == 0 && !hasLegacyRuleOutcomes {
			return fmt.Errorf("step %q rule requires at least one outcome route", step.ID)
		}
	case "timer":
		if !hasStringValue(cfg, "duration") {
			return fmt.Errorf("step %q timer requires duration", step.ID)
		}
	case "notification":
		if !hasStringValue(cfg, "channel") {
			return fmt.Errorf("step %q notification requires channel", step.ID)
		}
	case "ai_component":
		if !hasStringValue(cfg, "component") {
			return fmt.Errorf("step %q ai_component requires component", step.ID)
		}
	case "extraction":
		if !hasStringValue(cfg, "document_path") && !hasStringValue(cfg, "document_ref") {
			return fmt.Errorf("step %q extraction requires document_path or document_ref", step.ID)
		}
		if !hasStringValue(cfg, "schema") && !hasStringValue(cfg, "schema_name") && !hasStringValue(cfg, "schema_id") {
			return fmt.Errorf("step %q extraction requires schema, schema_name, or schema_id", step.ID)
		}
		if !hasStringValue(cfg, "output_path") {
			return fmt.Errorf("step %q extraction requires output_path", step.ID)
		}
	}
	return nil
}

func validateStepExpressions(evaluator *expressions.Evaluator, step engine.WorkflowStep) error {
	if err := validateBooleanExpression(evaluator, step.Condition, fmt.Sprintf("step %q condition", step.ID)); err != nil {
		return err
	}
	if strings.TrimSpace(step.Type) != "rule" {
		return nil
	}
	cfg, err := decodeStepConfig(step)
	if err != nil {
		return err
	}
	rawOutcomes, _ := cfg["outcomes"].([]any)
	for _, raw := range rawOutcomes {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(item["name"]))
		condition := strings.TrimSpace(fmt.Sprint(item["condition"]))
		if condition == "" {
			continue
		}
		location := fmt.Sprintf("step %q rule outcome %q condition", step.ID, name)
		if err := validateBooleanExpression(evaluator, condition, location); err != nil {
			return err
		}
	}
	if mappedOutcomes, ok := cfg["outcomes"].(map[string]any); ok {
		for name, raw := range mappedOutcomes {
			item, ok := raw.(map[string]any)
			if !ok {
				condition := strings.TrimSpace(fmt.Sprint(raw))
				if condition == "" {
					continue
				}
				location := fmt.Sprintf("step %q rule outcome %q condition", step.ID, strings.TrimSpace(name))
				if err := validateBooleanExpression(evaluator, condition, location); err != nil {
					return err
				}
				continue
			}
			condition := strings.TrimSpace(fmt.Sprint(item["condition"]))
			if condition == "" {
				continue
			}
			location := fmt.Sprintf("step %q rule outcome %q condition", step.ID, strings.TrimSpace(name))
			if err := validateBooleanExpression(evaluator, condition, location); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBooleanExpression(evaluator *expressions.Evaluator, expr string, location string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	result, err := evaluator.Evaluate(expr, expressionValidationContext())
	if err != nil {
		if shouldIgnoreExpressionEvaluationError(err) {
			return nil
		}
		return fmt.Errorf("%s is invalid: %w", location, err)
	}
	if _, ok := result.(bool); !ok {
		return fmt.Errorf("%s must evaluate to boolean", location)
	}
	return nil
}

func shouldIgnoreExpressionEvaluationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cannot read property") ||
		strings.Contains(msg, "is undefined") ||
		strings.Contains(msg, "undefined at")
}

func expressionValidationContext() map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339)
	return map[string]interface{}{
		"now": now,
		"case": map[string]any{
			"id":   "validation-case",
			"data": map[string]any{},
		},
		"steps": map[string]any{},
	}
}

func decodeStepConfig(step engine.WorkflowStep) (map[string]any, error) {
	if len(step.Config) == 0 {
		return map[string]any{}, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(step.Config, &cfg); err != nil {
		return nil, fmt.Errorf("step %q config must be a JSON object: %w", step.ID, err)
	}
	if cfg == nil {
		return map[string]any{}, nil
	}
	return cfg, nil
}

func hasKey(cfg map[string]any, key string) bool {
	_, ok := cfg[key]
	return ok
}

func hasStringValue(cfg map[string]any, key string) bool {
	raw, ok := cfg[key]
	if !ok {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(raw)) != ""
}

func validateAgentStepConfig(stepID string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("step %q agent config must be a JSON object: %w", stepID, err)
	}
	if _, hasLegacyContextSources := cfg["context_sources"]; hasLegacyContextSources {
		return fmt.Errorf("step %q uses unsupported key \"context_sources\"; use \"context\"", stepID)
	}
	if ctxRaw, ok := cfg["context"]; ok {
		if _, ok := ctxRaw.([]any); !ok {
			return fmt.Errorf("step %q field \"context\" must be an array", stepID)
		}
	}
	if schemaRaw, ok := cfg["output_schema"]; ok {
		schemaObj, ok := schemaRaw.(map[string]any)
		if !ok || schemaObj == nil {
			return fmt.Errorf("step %q field \"output_schema\" must be an object", stepID)
		}
	}
	if actionRaw, ok := cfg["on_low_confidence"]; ok {
		action, ok := actionRaw.(string)
		if !ok {
			return fmt.Errorf("step %q field \"on_low_confidence\" must be a string", stepID)
		}
		action = strings.TrimSpace(action)
		if action != "" && action != agentLowConfidenceEscalate {
			return fmt.Errorf("step %q field \"on_low_confidence\" must be %q", stepID, agentLowConfidenceEscalate)
		}
	}
	return nil
}

func validateAIComponentStepConfig(stepID string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("step %q ai_component config must be a JSON object: %w", stepID, err)
	}
	if in, ok := cfg["input_paths"]; ok {
		if _, ok := in.(map[string]any); !ok {
			return fmt.Errorf("step %q field \"input_paths\" must be an object", stepID)
		}
	}
	if out, ok := cfg["config_values"]; ok {
		if _, ok := out.(map[string]any); !ok {
			return fmt.Errorf("step %q field \"config_values\" must be an object", stepID)
		}
	}
	return nil
}

func validateExtractionStepConfig(stepID string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("step %q extraction config must be a JSON object: %w", stepID, err)
	}
	if err := validateThresholdRange(cfg, "auto_accept_threshold", stepID); err != nil {
		return err
	}
	if err := validateThresholdRange(cfg, "review_threshold", stepID); err != nil {
		return err
	}
	autoAccept, hasAuto := asFloat(cfg["auto_accept_threshold"])
	review, hasReview := asFloat(cfg["review_threshold"])
	if hasAuto && hasReview && review > autoAccept {
		return fmt.Errorf("step %q review_threshold cannot exceed auto_accept_threshold", stepID)
	}
	if onReview, ok := cfg["on_review"]; ok {
		obj, ok := onReview.(map[string]any)
		if !ok {
			return fmt.Errorf("step %q field \"on_review\" must be an object", stepID)
		}
		if hasKey(obj, "sla_hours") {
			sla, ok := asFloat(obj["sla_hours"])
			if !ok || sla <= 0 {
				return fmt.Errorf("step %q field \"on_review.sla_hours\" must be > 0", stepID)
			}
		}
	}
	if onReject, ok := cfg["on_reject"]; ok {
		if _, ok := onReject.(map[string]any); !ok {
			return fmt.Errorf("step %q field \"on_reject\" must be an object", stepID)
		}
	}
	return nil
}

func validateThresholdRange(cfg map[string]any, key string, stepID string) error {
	value, ok := cfg[key]
	if !ok {
		return nil
	}
	threshold, ok := asFloat(value)
	if !ok || threshold < 0 || threshold > 1 {
		return fmt.Errorf("step %q field %q must be a number between 0 and 1", stepID, key)
	}
	return nil
}

func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
