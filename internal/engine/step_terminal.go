package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (e *Engine) CompleteStep(ctx context.Context, caseID uuid.UUID, stepID string, result *StepResult) error {
	return e.completeStep(ctx, caseID, stepID, result)
}

func (e *Engine) completeStep(ctx context.Context, caseID uuid.UUID, stepID string, result *StepResult) error {
	if result == nil {
		result = &StepResult{}
	}

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin complete step tx: %w", err)
	}
	defer func() { _ = e.auditSvc.RollbackTx(tx) }()

	var caseStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM cases WHERE id = $1 FOR UPDATE`, caseID).Scan(&caseStatus); err != nil {
		return fmt.Errorf("lock case for complete step: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal step result: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
	UPDATE case_steps
	SET
	    state = 'completed',
    completed_at = now(),
    result = $3::jsonb,
    error = NULL,
    events = COALESCE(events, '[]'::jsonb)
      || CASE
            WHEN NULLIF($5, '') IS NULL THEN '[]'::jsonb
            ELSE jsonb_build_array($5::jsonb)
         END
      || jsonb_build_array(
        jsonb_build_object('type', 'completed', 'attempts', $4::int, 'at', now())
	    )
	WHERE case_id = $1 AND step_id = $2 AND state = 'active'
	`, caseID, stepID, string(resultJSON), result.Attempts, string(result.ExecutionEvent))
	if err != nil {
		return fmt.Errorf("update completed step state: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrStepNotActive
	}

	if result.WritesCaseData && len(result.CaseDataPatch) > 0 {
		if _, err := tx.ExecContext(ctx, `
UPDATE cases
SET
    data = COALESCE(data, '{}'::jsonb) || $2::jsonb,
    version = version + 1,
    updated_at = now()
WHERE id = $1
`, caseID, string(result.CaseDataPatch)); err != nil {
			return fmt.Errorf("write step output to case data: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
			return fmt.Errorf("touch case for complete step: %w", err)
		}
	}

	eventType := "step"
	action := "completed"
	if result.AuditEventType != "" {
		for _, sep := range []string{".", "/"} {
			if strings.Contains(result.AuditEventType, sep) {
				parts := strings.SplitN(result.AuditEventType, sep, 2)
				if parts[0] != "" && parts[1] != "" {
					eventType = parts[0]
					action = parts[1]
					break
				}
			}
		}
		if !strings.Contains(result.AuditEventType, ".") && !strings.Contains(result.AuditEventType, "/") {
			eventType = result.AuditEventType
		}
	}
	if err := e.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, eventType, e.systemActor(), "system", action, map[string]any{"attempts": result.Attempts, "outcome": result.Outcome}); err != nil {
		return err
	}
	if err := e.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit complete step: %w", err)
	}
	tenantID, terr := e.lookupTenantID(ctx, caseID)
	if terr == nil {
		e.updateCaseStepStateMetrics(ctx, tenantID)
	}
	slog.InfoContext(ctx, "step completed",
		append(observability.RequestAttrs(ctx),
			"case_id", caseID.String(),
			"step_id", stepID,
		)...,
	)

	if caseStatus != "cancelled" {
		e.triggerEvaluation(caseID)
	}
	return nil
}

func (e *Engine) FailStep(ctx context.Context, caseID uuid.UUID, stepID string, failErr error) error {
	return e.failStep(ctx, caseID, stepID, failErr)
}

func (e *Engine) failStep(ctx context.Context, caseID uuid.UUID, stepID string, failErr error) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fail step tx: %w", err)
	}
	defer func() { _ = e.auditSvc.RollbackTx(tx) }()

	var caseStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM cases WHERE id = $1 FOR UPDATE`, caseID).Scan(&caseStatus); err != nil {
		return fmt.Errorf("lock case for fail step: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET
    state = 'failed',
    completed_at = now(),
    error = jsonb_build_object('message', $3),
    events = COALESCE(events, '[]'::jsonb) || jsonb_build_array(
        jsonb_build_object('type', 'failed', 'error', $3, 'at', now())
    )
WHERE case_id = $1 AND step_id = $2 AND state = 'active'
`, caseID, stepID, failErr.Error()); err != nil {
		return fmt.Errorf("update failed step state: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case for fail step: %w", err)
	}
	if err := e.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "step", e.systemActor(), "system", "failed", map[string]any{"error": failErr.Error()}); err != nil {
		return err
	}
	if err := e.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit fail step: %w", err)
	}
	tenantID, terr := e.lookupTenantID(ctx, caseID)
	if terr == nil {
		e.updateCaseStepStateMetrics(ctx, tenantID)
	}
	slog.ErrorContext(ctx, "step failed",
		append(observability.RequestAttrs(ctx),
			"case_id", caseID.String(),
			"step_id", stepID,
			"error", failErr,
		)...,
	)

	if caseStatus != "cancelled" {
		e.triggerEvaluation(caseID)
	}
	return nil
}

func (e *Engine) skipStepTerminal(ctx context.Context, caseID uuid.UUID, stepID string, attempts int, cause error) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin skip step tx: %w", err)
	}
	defer func() { _ = e.auditSvc.RollbackTx(tx) }()

	if _, err := tx.ExecContext(ctx, `
	UPDATE case_steps
	SET
	    state = 'skipped',
	    completed_at = now(),
	    error = jsonb_build_object('message', $3::text),
	    events = COALESCE(events, '[]'::jsonb) || jsonb_build_array(
	        jsonb_build_object('type', 'skipped_on_exhausted', 'attempts', $4::int, 'error', $3::text, 'at', now())
	    )
WHERE case_id = $1 AND step_id = $2 AND state = 'active'
`, caseID, stepID, cause.Error(), attempts); err != nil {
		return fmt.Errorf("mark step skipped on exhausted policy: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case for skip-on-exhausted: %w", err)
	}
	if err := e.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "step", e.systemActor(), "system", "skipped", map[string]any{"attempts": attempts, "error": cause.Error()}); err != nil {
		return err
	}
	if err := e.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit skip step terminal: %w", err)
	}
	tenantID, terr := e.lookupTenantID(ctx, caseID)
	if terr == nil {
		e.updateCaseStepStateMetrics(ctx, tenantID)
	}
	return nil
}

// UpdateCaseData performs optimistic locking update of cases.data and increments version.
func (e *Engine) UpdateCaseData(ctx context.Context, caseID uuid.UUID, expectedVersion int, patch json.RawMessage) error {
	res, err := e.db.ExecContext(ctx, `
UPDATE cases
SET data = COALESCE(data, '{}'::jsonb) || $3::jsonb,
    version = version + 1,
    updated_at = now()
WHERE id = $1 AND version = $2
`, caseID, expectedVersion, string(patch))
	if err != nil {
		return fmt.Errorf("optimistic update case data: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected optimistic update: %w", err)
	}
	if affected == 0 {
		return ErrCaseDataConflict
	}
	return nil
}
