package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func (e *Engine) Recover(ctx context.Context) error {
	rows, err := e.db.QueryContext(ctx, `
SELECT DISTINCT cs.case_id
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE cs.state = 'active' AND c.status != 'cancelled'
`)
	if err != nil {
		return fmt.Errorf("query active cases for recovery: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var caseID uuid.UUID
		if err := rows.Scan(&caseID); err != nil {
			return fmt.Errorf("scan recovery case id: %w", err)
		}
		if err := e.recoverCase(ctx, caseID); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate recovery cases: %w", err)
	}
	return nil
}

func (e *Engine) recoverCase(ctx context.Context, caseID uuid.UUID) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recover case tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ast, err := loadWorkflowASTTx(ctx, tx, caseID)
	if err != nil {
		return err
	}
	stepsByID := stepMap(ast)

	rows, err := tx.QueryContext(ctx, `
SELECT step_id, result
FROM case_steps
WHERE case_id = $1 AND state = 'active'
`, caseID)
	if err != nil {
		return fmt.Errorf("query active steps for recovery: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type recStep struct {
		stepID string
		result []byte
	}
	active := make([]recStep, 0)
	for rows.Next() {
		var s recStep
		if err := rows.Scan(&s.stepID, &s.result); err != nil {
			return fmt.Errorf("scan active step for recovery: %w", err)
		}
		active = append(active, s)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recover case preload: %w", err)
	}

	for _, s := range active {
		step, ok := stepsByID[s.stepID]
		if !ok {
			continue
		}
		switch step.Type {
		case "integration":
			if idempotentStep(step) {
				e.dispatchStep(caseID, step)
			} else {
				_ = e.failStep(ctx, caseID, step.ID, errors.New("recovery failed non-idempotent integration step"))
			}
		case "agent":
			if len(s.result) > 0 {
				_ = e.completeStep(ctx, caseID, step.ID, &StepResult{Output: s.result})
			} else {
				e.dispatchStep(caseID, step)
			}
		case "human_task":
			// no action, remains active
		case "rule":
			e.dispatchStep(caseID, step)
		case "timer":
			e.dispatchStep(caseID, step)
		default:
			e.dispatchStep(caseID, step)
		}
	}

	e.triggerEvaluation(caseID)
	return nil
}

func idempotentStep(step WorkflowStep) bool {
	if step.Metadata == nil {
		return false
	}
	v, ok := step.Metadata["idempotent"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
