package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (e *Engine) CancelCase(ctx context.Context, caseID uuid.UUID, actorID uuid.UUID, reason string) error {
	return e.cancelCase(ctx, caseID, actorID, reason)
}

func (e *Engine) cancelCase(ctx context.Context, caseID uuid.UUID, actorID uuid.UUID, reason string) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cancel case tx: %w", err)
	}
	defer func() { _ = e.auditSvc.RollbackTx(tx) }()

	ast, err := loadWorkflowASTTx(ctx, tx, caseID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `SELECT id FROM cases WHERE id = $1 FOR UPDATE`, caseID); err != nil {
		return fmt.Errorf("lock case for cancellation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE cases
SET status = 'cancelled', version = version + 1, updated_at = now()
WHERE id = $1
`, caseID); err != nil {
		return fmt.Errorf("set case cancelled: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET state = 'skipped', completed_at = now()
WHERE case_id = $1 AND state IN ('pending', 'ready')
`, caseID); err != nil {
		return fmt.Errorf("skip pending/ready steps on cancellation: %w", err)
	}

	for _, step := range ast.Steps {
		if step.Type != "human_task" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET
    state = 'skipped',
    completed_at = now(),
    metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('cancelled_task', true)
WHERE case_id = $1 AND state = 'active' AND step_id = $2
`, caseID, step.ID); err != nil {
			return fmt.Errorf("cancel active human task %s: %w", step.ID, err)
		}
	}

	if err := e.auditSvc.RecordCaseEventTx(ctx, tx, caseID, "", "case", actorID, "human", "cancelled", map[string]any{"reason": reason}); err != nil {
		return err
	}
	if err := e.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit cancel case: %w", err)
	}
	tenantID, terr := e.lookupTenantID(ctx, caseID)
	if terr == nil {
		e.updateCaseStepStateMetrics(ctx, tenantID)
	}
	slog.InfoContext(ctx, "case cancelled in engine",
		append(observability.RequestAttrs(ctx),
			"case_id", caseID.String(),
			"actor_id", actorID.String(),
		)...,
	)
	return nil
}
