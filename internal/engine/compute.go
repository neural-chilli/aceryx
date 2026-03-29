package engine

import (
	"encoding/json"
	"fmt"
)

func computeTransitions(ast WorkflowAST, states map[string]StepState, evaluator ExpressionEvaluator, ctx map[string]interface{}) ([]Transition, error) {
	if err := ValidateAST(ast); err != nil {
		return nil, err
	}

	stepByID := stepMap(ast)
	reverse := reverseDependencies(ast)
	virtual := make(map[string]StepState, len(states))
	for k, v := range states {
		virtual[k] = v
	}
	for _, step := range ast.Steps {
		if _, ok := virtual[step.ID]; !ok {
			virtual[step.ID] = StepState{StepID: step.ID, State: StatePending}
		}
	}

	transitions := make([]Transition, 0)
	seen := make(map[string]bool)

	for {
		changed := false

		for _, source := range ast.Steps {
			srcState := virtual[source.ID]
			if srcState.State != StateCompleted {
				continue
			}

			selectedOutcome := extractOutcome(srcState.Result)
			if len(source.Outcomes) == 0 {
				continue
			}

			allowed := make(map[string]bool)
			for _, sid := range source.Outcomes[selectedOutcome] {
				allowed[sid] = true
			}

			allOutcomeTargets := make(map[string]bool)
			for _, list := range source.Outcomes {
				for _, sid := range list {
					allOutcomeTargets[sid] = true
				}
			}

			for _, dependentID := range reverse[source.ID] {
				if !allOutcomeTargets[dependentID] {
					continue
				}
				if allowed[dependentID] {
					continue
				}
				depState := virtual[dependentID]
				if depState.State != StatePending && depState.State != StateReady {
					continue
				}
				key := dependentID + ":" + string(TransitionToSkipped)
				if seen[key] {
					continue
				}
				seen[key] = true
				transitions = append(transitions, Transition{
					StepID:  dependentID,
					From:    depState.State,
					To:      StateSkipped,
					Type:    TransitionToSkipped,
					Reason:  ReasonOutcomeRouting,
					Outcome: selectedOutcome,
				})
				depState.State = StateSkipped
				virtual[dependentID] = depState
				changed = true
			}
		}

		for _, step := range ast.Steps {
			st := virtual[step.ID]
			if st.State != StatePending {
				continue
			}

			deps := step.DependsOn
			if len(deps) == 0 {
				key := step.ID + ":" + string(TransitionToReady)
				if !seen[key] {
					seen[key] = true
					transitions = append(transitions, Transition{
						StepID: step.ID,
						From:   StatePending,
						To:     StateReady,
						Type:   TransitionToReady,
						Reason: ReasonDependenciesSatisfied,
					})
					st.State = StateReady
					virtual[step.ID] = st
					changed = true
				}
				continue
			}

			terminalCount := 0
			completedCount := 0
			skippedCount := 0
			allTerminal := true
			for _, dep := range deps {
				depState := virtual[dep].State
				if isTerminal(depState) {
					terminalCount++
				}
				if depState == StateCompleted {
					completedCount++
				}
				if depState == StateSkipped {
					skippedCount++
				}
				if !isTerminal(depState) {
					allTerminal = false
				}
			}

			join := normalizedJoin(step.Join)
			switch join {
			case "all":
				if allTerminal && completedCount > 0 {
					key := step.ID + ":" + string(TransitionToReady)
					if !seen[key] {
						seen[key] = true
						transitions = append(transitions, Transition{StepID: step.ID, From: StatePending, To: StateReady, Type: TransitionToReady, Reason: ReasonDependenciesSatisfied})
						st.State = StateReady
						virtual[step.ID] = st
						changed = true
					}
				} else if allTerminal && skippedCount == len(deps) {
					key := step.ID + ":" + string(TransitionToSkipped)
					if !seen[key] {
						seen[key] = true
						transitions = append(transitions, Transition{StepID: step.ID, From: StatePending, To: StateSkipped, Type: TransitionToSkipped, Reason: ReasonSkipPropagation})
						st.State = StateSkipped
						virtual[step.ID] = st
						changed = true
					}
				}
			case "any":
				if terminalCount > 0 {
					key := step.ID + ":" + string(TransitionToReady)
					if !seen[key] {
						seen[key] = true
						transitions = append(transitions, Transition{StepID: step.ID, From: StatePending, To: StateReady, Type: TransitionToReady, Reason: ReasonJoinAnySatisfied})
						st.State = StateReady
						virtual[step.ID] = st
						changed = true
					}
				}
			default:
				return nil, fmt.Errorf("step %s: %w", step.ID, ErrInvalidJoinStrategy)
			}
		}

		for _, step := range ast.Steps {
			st := virtual[step.ID]
			if st.State != StateReady {
				continue
			}

			if step.Condition != "" {
				if evaluator == nil {
					return nil, fmt.Errorf("step %s has condition but evaluator is nil", step.ID)
				}
				ok, err := evaluator.EvaluateBool(step.Condition, ctx)
				if err != nil {
					return nil, fmt.Errorf("evaluate guard for step %s: %w", step.ID, err)
				}
				if !ok {
					key := step.ID + ":" + string(TransitionToSkipped)
					if !seen[key] {
						seen[key] = true
						transitions = append(transitions, Transition{StepID: step.ID, From: StateReady, To: StateSkipped, Type: TransitionToSkipped, Reason: ReasonGuardFalse})
						st.State = StateSkipped
						virtual[step.ID] = st
						changed = true
					}
					continue
				}
			}

			key := step.ID + ":" + string(TransitionToActive)
			if !seen[key] {
				seen[key] = true
				transitions = append(transitions, Transition{StepID: step.ID, From: StateReady, To: StateActive, Type: TransitionToActive, Reason: ReasonDependenciesSatisfied})
				st.State = StateActive
				virtual[step.ID] = st
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	_ = stepByID
	return transitions, nil
}

func isTerminal(state string) bool {
	return state == StateCompleted || state == StateFailed || state == StateSkipped
}

func extractOutcome(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var tmp struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(raw, &tmp); err != nil {
		return ""
	}
	return tmp.Outcome
}
