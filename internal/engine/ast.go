package engine

import (
	"encoding/json"
	"fmt"
)

func parseAST(raw []byte) (WorkflowAST, error) {
	var ast WorkflowAST
	if len(raw) == 0 {
		return ast, fmt.Errorf("empty workflow ast")
	}
	if err := json.Unmarshal(raw, &ast); err != nil {
		return ast, fmt.Errorf("unmarshal workflow ast: %w", err)
	}
	if err := ValidateAST(ast); err != nil {
		return ast, err
	}
	return ast, nil
}

func ValidateAST(ast WorkflowAST) error {
	if len(ast.Steps) == 0 {
		return fmt.Errorf("workflow has no steps")
	}

	byID := make(map[string]WorkflowStep, len(ast.Steps))
	for _, step := range ast.Steps {
		if step.ID == "" {
			return fmt.Errorf("workflow step id cannot be empty")
		}
		if _, exists := byID[step.ID]; exists {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		join := normalizedJoin(step.Join)
		if join != "all" && join != "any" {
			return fmt.Errorf("step %s: %w %q", step.ID, ErrInvalidJoinStrategy, step.Join)
		}
		byID[step.ID] = step
	}

	for _, step := range ast.Steps {
		for _, dep := range step.DependsOn {
			if _, ok := byID[dep]; !ok {
				return fmt.Errorf("step %s depends on unknown step %s", step.ID, dep)
			}
		}
	}

	state := make(map[string]int, len(ast.Steps))
	var visit func(string) error
	visit = func(stepID string) error {
		if state[stepID] == 1 {
			return fmt.Errorf("%w at %s", ErrCycleDetectedInAST, stepID)
		}
		if state[stepID] == 2 {
			return nil
		}
		state[stepID] = 1
		for _, dep := range byID[stepID].DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[stepID] = 2
		return nil
	}

	for _, step := range ast.Steps {
		if err := visit(step.ID); err != nil {
			return err
		}
	}

	return nil
}

func stepMap(ast WorkflowAST) map[string]WorkflowStep {
	byID := make(map[string]WorkflowStep, len(ast.Steps))
	for _, s := range ast.Steps {
		byID[s.ID] = s
	}
	return byID
}

func reverseDependencies(ast WorkflowAST) map[string][]string {
	rev := make(map[string][]string, len(ast.Steps))
	for _, step := range ast.Steps {
		for _, dep := range step.DependsOn {
			rev[dep] = append(rev[dep], step.ID)
		}
	}
	return rev
}

func normalizedJoin(join string) string {
	if join == "" {
		return "all"
	}
	return join
}
