package agents

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

func (a *AgentExecutor) createHumanReviewTask(ctx context.Context, caseID uuid.UUID, stepID string, cfg StepConfig, output map[string]any, confidence float64) error {
	if a.tasks == nil {
		return fmt.Errorf("task service not configured for low-confidence escalation")
	}
	fields := make([]tasks.FormField, 0, len(cfg.OutputSchema))
	keys := make([]string, 0, len(cfg.OutputSchema))
	for key := range cfg.OutputSchema {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		def := cfg.OutputSchema[key]
		fieldType := def.Type
		if fieldType == "" {
			fieldType = "text"
		}
		if fieldType == "number" {
			fieldType = "number"
		} else {
			fieldType = "text"
		}
		fields = append(fields, tasks.FormField{ID: key, Type: fieldType, Required: true, Bind: "decision." + key})
	}

	cfgTask := tasks.AssignmentConfig{
		AssignToRole: cfg.AssignToRole,
		AssignToUser: cfg.AssignToUser,
		SLAHours:     cfg.SLAHours,
		Escalation:   cfg.Escalation,
		Form:         "agent_review",
		FormSchema:   tasks.FormSchema{Fields: fields},
		Outcomes:     []string{"accept", "modify", "override"},
		Metadata: map[string]any{
			"agent_review": map[string]any{
				"original_output": output,
				"confidence":      confidence,
				"reasoning":       output["reasoning"],
				"flags":           output["flags"],
			},
		},
	}
	if cfgTask.AssignToRole == "" && cfgTask.AssignToUser == "" {
		cfgTask.AssignToRole = "case_worker"
	}
	if err := a.tasks.CreateTaskFromActivation(ctx, caseID, stepID, cfgTask); err != nil {
		return err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent escalation audit tx: %w", err)
	}
	defer func() { _ = a.auditSvc.RollbackTx(tx) }()

	actorID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	if err := a.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "agent", actorID, "system", "escalated", map[string]any{
		"confidence": confidence,
		"threshold":  cfg.ConfidenceThreshold,
	}); err != nil {
		return err
	}
	return a.auditSvc.CommitTx(tx)
}
