package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/ai"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type PublishValidationError struct {
	StepID     string `json:"stepId"`
	Field      string `json:"field"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type PublishValidationErrors struct {
	Errors []PublishValidationError `json:"errors"`
}

func (p *PublishValidationErrors) Error() string {
	if p == nil || len(p.Errors) == 0 {
		return "invalid workflow ast"
	}
	first := p.Errors[0]
	if first.Message == "" {
		return "invalid workflow ast"
	}
	return fmt.Sprintf("invalid workflow ast: %s", first.Message)
}

func (p *PublishValidationErrors) add(err PublishValidationError) {
	if p == nil {
		return
	}
	if strings.TrimSpace(err.Message) == "" || strings.TrimSpace(err.Code) == "" {
		return
	}
	p.Errors = append(p.Errors, err)
}

func (p *PublishValidationErrors) hasErrors() bool {
	return p != nil && len(p.Errors) > 0
}

type aiComponentCatalog interface {
	List(ctx context.Context, tenantID uuid.UUID) ([]*ai.AIComponentDef, error)
}

func validatePublishWorkflow(
	ctx context.Context,
	tenantID uuid.UUID,
	astRaw []byte,
	catalog aiComponentCatalog,
) error {
	var workflow engine.WorkflowAST
	if err := json.Unmarshal(astRaw, &workflow); err != nil {
		return fmt.Errorf("invalid workflow ast: decode workflow ast: %w", err)
	}

	validation := &PublishValidationErrors{Errors: make([]PublishValidationError, 0)}
	byID := make(map[string]engine.WorkflowStep, len(workflow.Steps))
	inDegree := make(map[string]int, len(workflow.Steps))
	outgoing := make(map[string][]string, len(workflow.Steps))

	for _, step := range workflow.Steps {
		stepID := strings.TrimSpace(step.ID)
		if stepID != "" {
			byID[stepID] = step
		}
		inDegree[stepID] = len(step.DependsOn)
		for _, dep := range step.DependsOn {
			outgoing[dep] = append(outgoing[dep], stepID)
		}
	}

	for _, step := range workflow.Steps {
		stepID := strings.TrimSpace(step.ID)
		stepType := strings.TrimSpace(step.Type)
		if !isSupportedStepType(stepType) {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "type",
				Code:    "UNKNOWN_STEP_TYPE",
				Message: fmt.Sprintf("Step type %q is not supported", stepType),
			})
		}

		cfg, cfgErr := decodeStepConfig(step)
		if cfgErr != nil {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: cfgErr.Error(),
			})
			continue
		}
		addMissingRequiredConfigErrors(validation, step, cfg)

		for _, dep := range step.DependsOn {
			if _, ok := byID[dep]; !ok {
				validation.add(PublishValidationError{
					StepID:  stepID,
					Field:   "depends_on",
					Code:    "DANGLING_EDGE",
					Message: fmt.Sprintf("Step %q depends on unknown step %q", stepID, dep),
				})
			}
		}
		for outcome, targets := range step.Outcomes {
			for _, target := range targets {
				outgoing[stepID] = append(outgoing[stepID], target)
				if _, ok := byID[target]; !ok {
					validation.add(PublishValidationError{
						StepID:  stepID,
						Field:   "outcomes",
						Code:    "DANGLING_EDGE",
						Message: fmt.Sprintf("Step %q outcome %q targets unknown step %q", stepID, outcome, target),
					})
				}
			}
		}
	}

	addGraphErrors(validation, workflow, byID, inDegree, outgoing)
	if err := validateComponentRefs(ctx, tenantID, workflow, catalog, validation); err != nil {
		return err
	}

	if err := engine.ValidateAST(workflow); err != nil {
		if errors.Is(err, engine.ErrCycleDetectedInAST) {
			validation.add(PublishValidationError{
				Code:    "GRAPH_CYCLE",
				Message: "Workflow graph contains a cycle",
			})
		} else {
			return fmt.Errorf("invalid workflow ast: %w", err)
		}
	}

	if validation.hasErrors() {
		return validation
	}

	if err := validateWorkflowAST(astRaw); err != nil {
		return err
	}
	return nil
}

func validateComponentRefs(
	ctx context.Context,
	tenantID uuid.UUID,
	workflow engine.WorkflowAST,
	catalog aiComponentCatalog,
	validation *PublishValidationErrors,
) error {
	if catalog == nil {
		return nil
	}
	items, err := catalog.List(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list ai components: %w", err)
	}
	known := make([]string, 0, len(items))
	knownSet := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		known = append(known, id)
		knownSet[id] = struct{}{}
	}
	sort.Strings(known)

	for _, step := range workflow.Steps {
		if strings.TrimSpace(step.Type) != "ai_component" {
			continue
		}
		cfg, err := decodeStepConfig(step)
		if err != nil {
			continue
		}
		componentID := strings.TrimSpace(fmt.Sprint(cfg["component"]))
		if componentID == "" {
			continue
		}
		if _, ok := knownSet[componentID]; ok {
			continue
		}
		suggestions := suggestComponentIDs(componentID, known)
		suggestionText := ""
		if len(suggestions) > 0 {
			suggestionText = "Use one of: " + strings.Join(suggestions, ", ")
		}
		validation.add(PublishValidationError{
			StepID:     strings.TrimSpace(step.ID),
			Field:      "config.component",
			Code:       "INVALID_COMPONENT_REF",
			Message:    fmt.Sprintf("Component %s is not registered", componentID),
			Suggestion: suggestionText,
		})
	}
	return nil
}

func suggestComponentIDs(missing string, known []string) []string {
	if len(known) == 0 {
		return []string{"document_extraction", "sentiment_analysis"}
	}
	lowerMissing := strings.ToLower(strings.TrimSpace(missing))
	ranked := make([]string, 0, len(known))
	for _, id := range known {
		lower := strings.ToLower(id)
		if strings.Contains(lower, lowerMissing) || strings.Contains(lowerMissing, lower) {
			ranked = append(ranked, id)
		}
	}
	if len(ranked) == 0 {
		for _, id := range known {
			if strings.HasPrefix(id, "document_") || strings.HasPrefix(id, "sentiment_") {
				ranked = append(ranked, id)
			}
			if len(ranked) == 2 {
				return ranked
			}
		}
	}
	if len(ranked) == 0 {
		ranked = append(ranked, known...)
	}
	if len(ranked) > 2 {
		ranked = ranked[:2]
	}
	return ranked
}

func isSupportedStepType(stepType string) bool {
	switch strings.TrimSpace(stepType) {
	case "human_task", "agent", "ai_component", "extraction", "integration", "rule", "timer", "notification":
		return true
	default:
		return false
	}
}

func addMissingRequiredConfigErrors(validation *PublishValidationErrors, step engine.WorkflowStep, cfg map[string]any) {
	stepID := strings.TrimSpace(step.ID)
	switch strings.TrimSpace(step.Type) {
	case "human_task":
		if !hasStringValue(cfg, "assignee") && !hasKey(cfg, "candidate_roles") && !hasStringValue(cfg, "assign_to_role") && !hasStringValue(cfg, "assign_to_user") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.assignee",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q human_task requires assignee or candidate_roles", stepID),
			})
		}
		if !hasKey(cfg, "form") && !hasKey(cfg, "form_schema") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.form",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q human_task requires form", stepID),
			})
		}
	case "agent":
		if !hasStringValue(cfg, "prompt_template") && !hasStringValue(cfg, "prompt") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.prompt_template",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q agent requires prompt_template or prompt", stepID),
			})
		}
		if _, ok := cfg["output_schema"].(map[string]any); !ok {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.output_schema",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q agent requires output_schema object", stepID),
			})
		}
	case "ai_component":
		if !hasStringValue(cfg, "component") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.component",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q ai_component requires component", stepID),
			})
		}
	case "extraction":
		hasSchema := hasStringValue(cfg, "schema_id") || hasStringValue(cfg, "schema_name") || hasStringValue(cfg, "schema")
		if !hasSchema {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.schema_name",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q extraction requires schema_id or schema_name", stepID),
			})
		}
		hasDocument := hasStringValue(cfg, "document_ref") || hasStringValue(cfg, "document_path")
		if !hasDocument {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.document_ref",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q extraction requires document_ref", stepID),
			})
		}
		if !hasStringValue(cfg, "output_path") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.output_path",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q extraction requires output_path", stepID),
			})
		}
	case "integration":
		if !hasStringValue(cfg, "connector") || !hasStringValue(cfg, "action") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.connector",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q integration requires connector and action", stepID),
			})
		}
	case "rule":
		hasRuleExpression := hasStringValue(cfg, "expression") || strings.TrimSpace(step.Condition) != ""
		if !hasRuleExpression {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.expression",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q rule requires expression or guard condition", stepID),
			})
		}
		if len(step.Outcomes) == 0 {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "outcomes",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q rule requires outcomes", stepID),
			})
		}
	case "timer":
		if !hasStringValue(cfg, "duration") && !hasStringValue(cfg, "schedule") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.duration",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q timer requires duration or schedule", stepID),
			})
		}
	case "notification":
		if !hasStringValue(cfg, "channel") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.channel",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q notification requires channel", stepID),
			})
		}
		if !hasStringValue(cfg, "template") && !hasStringValue(cfg, "message") {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "config.template",
				Code:    "MISSING_REQUIRED_CONFIG",
				Message: fmt.Sprintf("Step %q notification requires template or message", stepID),
			})
		}
	}
}

func addGraphErrors(
	validation *PublishValidationErrors,
	workflow engine.WorkflowAST,
	byID map[string]engine.WorkflowStep,
	inDegree map[string]int,
	outgoing map[string][]string,
) {
	if len(workflow.Steps) == 0 {
		return
	}
	roots := make([]string, 0)
	for _, step := range workflow.Steps {
		stepID := strings.TrimSpace(step.ID)
		if inDegree[stepID] == 0 {
			roots = append(roots, stepID)
		}
	}
	if len(roots) == 0 {
		validation.add(PublishValidationError{
			Field:   "steps",
			Code:    "INVALID_ENTRYPOINT",
			Message: "Workflow must contain at least one entrypoint step with no dependencies",
		})
	}

	visited := map[string]struct{}{}
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		for _, target := range outgoing[current] {
			if _, ok := byID[target]; ok {
				queue = append(queue, target)
			}
		}
	}
	for _, step := range workflow.Steps {
		stepID := strings.TrimSpace(step.ID)
		if _, ok := visited[stepID]; !ok {
			validation.add(PublishValidationError{
				StepID:  stepID,
				Field:   "depends_on",
				Code:    "UNREACHABLE_NODE",
				Message: fmt.Sprintf("Step %q is unreachable from workflow entrypoint", stepID),
			})
		}
	}

	if len(workflow.Steps) > 0 {
		if isDisconnected(workflow) {
			validation.add(PublishValidationError{
				Field:   "steps",
				Code:    "DISCONNECTED_GRAPH",
				Message: "Workflow graph is disconnected",
			})
		}
	}

	terminalCount := 0
	for _, step := range workflow.Steps {
		stepID := strings.TrimSpace(step.ID)
		targets := append([]string(nil), outgoing[stepID]...)
		targets = slices.DeleteFunc(targets, func(item string) bool {
			_, ok := byID[item]
			return !ok
		})
		if len(targets) == 0 {
			terminalCount++
		}
	}
	if terminalCount == 0 {
		validation.add(PublishValidationError{
			Field:   "steps",
			Code:    "INVALID_TERMINAL",
			Message: "Workflow must contain at least one terminal step",
		})
	}
}

func isDisconnected(workflow engine.WorkflowAST) bool {
	if len(workflow.Steps) == 0 {
		return false
	}
	neighbors := make(map[string][]string, len(workflow.Steps))
	for _, step := range workflow.Steps {
		stepID := strings.TrimSpace(step.ID)
		for _, dep := range step.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			neighbors[stepID] = append(neighbors[stepID], dep)
			neighbors[dep] = append(neighbors[dep], stepID)
		}
		for _, targets := range step.Outcomes {
			for _, target := range targets {
				target = strings.TrimSpace(target)
				if target == "" {
					continue
				}
				neighbors[stepID] = append(neighbors[stepID], target)
				neighbors[target] = append(neighbors[target], stepID)
			}
		}
	}
	start := strings.TrimSpace(workflow.Steps[0].ID)
	visited := map[string]struct{}{}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		queue = append(queue, neighbors[current]...)
	}
	return len(visited) != len(workflow.Steps)
}
