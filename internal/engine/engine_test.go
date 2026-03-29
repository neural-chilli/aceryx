package engine

import (
	"testing"
)

type fixedEval struct {
	values map[string]bool
}

func (f fixedEval) EvaluateBool(expr string, _ map[string]interface{}) (bool, error) {
	if v, ok := f.values[expr]; ok {
		return v, nil
	}
	return false, nil
}

func TestComputeTransitions_Table(t *testing.T) {
	tests := []struct {
		name       string
		ast        WorkflowAST
		states     map[string]StepState
		evaluator  ExpressionEvaluator
		wantActive []string
		wantSkip   []string
	}{
		{
			name: "all dependencies completed to ready active",
			ast: WorkflowAST{Steps: []WorkflowStep{
				{ID: "a", Type: "rule"},
				{ID: "b", Type: "rule", DependsOn: []string{"a"}},
			}},
			states: map[string]StepState{
				"a": {StepID: "a", State: StateCompleted},
				"b": {StepID: "b", State: StatePending},
			},
			wantActive: []string{"b"},
		},
		{
			name: "mixed completed skipped join all activates",
			ast: WorkflowAST{Steps: []WorkflowStep{
				{ID: "a", Type: "rule"},
				{ID: "b", Type: "rule"},
				{ID: "c", Type: "rule", DependsOn: []string{"a", "b"}, Join: "all"},
			}},
			states: map[string]StepState{
				"a": {StepID: "a", State: StateCompleted},
				"b": {StepID: "b", State: StateSkipped},
				"c": {StepID: "c", State: StatePending},
			},
			wantActive: []string{"c"},
		},
		{
			name: "all skipped join all propagates skip",
			ast: WorkflowAST{Steps: []WorkflowStep{
				{ID: "a", Type: "rule"},
				{ID: "b", Type: "rule"},
				{ID: "c", Type: "rule", DependsOn: []string{"a", "b"}, Join: "all"},
			}},
			states: map[string]StepState{
				"a": {StepID: "a", State: StateSkipped},
				"b": {StepID: "b", State: StateSkipped},
				"c": {StepID: "c", State: StatePending},
			},
			wantSkip: []string{"c"},
		},
		{
			name: "one terminal join any activates",
			ast: WorkflowAST{Steps: []WorkflowStep{
				{ID: "a", Type: "rule"},
				{ID: "b", Type: "rule"},
				{ID: "c", Type: "rule", DependsOn: []string{"a", "b"}, Join: "any"},
			}},
			states: map[string]StepState{
				"a": {StepID: "a", State: StateCompleted},
				"b": {StepID: "b", State: StatePending},
				"c": {StepID: "c", State: StatePending},
			},
			wantActive: []string{"c"},
		},
		{
			name: "guard false becomes skipped",
			ast: WorkflowAST{Steps: []WorkflowStep{
				{ID: "a", Type: "rule", Condition: "case.amount > 100"},
			}},
			states: map[string]StepState{
				"a": {StepID: "a", State: StateReady},
			},
			evaluator: fixedEval{values: map[string]bool{"case.amount > 100": false}},
			wantSkip:  []string{"a"},
		},
		{
			name: "skip propagation chain",
			ast: WorkflowAST{Steps: []WorkflowStep{
				{ID: "a", Type: "rule"},
				{ID: "b", Type: "rule", DependsOn: []string{"a"}, Join: "all"},
				{ID: "c", Type: "rule", DependsOn: []string{"b"}, Join: "all"},
			}},
			states: map[string]StepState{
				"a": {StepID: "a", State: StateSkipped},
				"b": {StepID: "b", State: StatePending},
				"c": {StepID: "c", State: StatePending},
			},
			wantSkip: []string{"b", "c"},
		},
		{
			name: "diamond mixed join modes",
			ast: WorkflowAST{Steps: []WorkflowStep{
				{ID: "a", Type: "rule"},
				{ID: "b", Type: "rule", DependsOn: []string{"a"}},
				{ID: "c", Type: "rule", DependsOn: []string{"a"}},
				{ID: "d", Type: "rule", DependsOn: []string{"b", "c"}, Join: "any"},
			}},
			states: map[string]StepState{
				"a": {StepID: "a", State: StateCompleted},
				"b": {StepID: "b", State: StateCompleted},
				"c": {StepID: "c", State: StatePending},
				"d": {StepID: "d", State: StatePending},
			},
			wantActive: []string{"d"},
		},
		{
			name: "outcome routing skips non selected branch",
			ast: WorkflowAST{Steps: []WorkflowStep{
				{ID: "router", Type: "rule", Outcomes: map[string][]string{
					"approved": {"approve_step"},
					"rejected": {"reject_step"},
				}},
				{ID: "approve_step", Type: "rule", DependsOn: []string{"router"}},
				{ID: "reject_step", Type: "rule", DependsOn: []string{"router"}},
			}},
			states: map[string]StepState{
				"router":       {StepID: "router", State: StateCompleted, Result: []byte(`{"outcome":"approved"}`)},
				"approve_step": {StepID: "approve_step", State: StatePending},
				"reject_step":  {StepID: "reject_step", State: StatePending},
			},
			wantActive: []string{"approve_step"},
			wantSkip:   []string{"reject_step"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := tt.evaluator
			if evaluator == nil {
				evaluator = fixedEval{values: map[string]bool{}}
			}
			got, err := computeTransitions(tt.ast, tt.states, evaluator, map[string]interface{}{"case": map[string]interface{}{}})
			if err != nil {
				t.Fatalf("computeTransitions() error = %v", err)
			}

			active := make(map[string]bool)
			skipped := make(map[string]bool)
			for _, tr := range got {
				if tr.To == StateActive {
					active[tr.StepID] = true
				}
				if tr.To == StateSkipped {
					skipped[tr.StepID] = true
				}
			}

			for _, stepID := range tt.wantActive {
				if !active[stepID] {
					t.Fatalf("expected step %s to activate, transitions=%v", stepID, got)
				}
			}
			for _, stepID := range tt.wantSkip {
				if !skipped[stepID] {
					t.Fatalf("expected step %s to skip, transitions=%v", stepID, got)
				}
			}
		})
	}
}

func TestValidateAST_CycleDetection(t *testing.T) {
	ast := WorkflowAST{Steps: []WorkflowStep{
		{ID: "a", Type: "rule", DependsOn: []string{"b"}},
		{ID: "b", Type: "rule", DependsOn: []string{"a"}},
	}}

	err := ValidateAST(ast)
	if err == nil {
		t.Fatal("expected cycle validation error")
	}
}
