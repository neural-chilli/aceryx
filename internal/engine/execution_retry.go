package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (e *Engine) executeWithRetry(ctx context.Context, caseID uuid.UUID, step WorkflowStep) error {
	start := time.Now()
	exec, err := e.executorFor(step.Type)
	if err != nil {
		return err
	}
	policy := defaultErrorPolicyForStep(step.Type, step.ErrorPolicy)

	attempt := 0
	for {
		attempt++
		result, execErr := exec.Execute(ctx, caseID, step.ID, step.Config)
		if errors.Is(execErr, ErrStepAwaitingReview) {
			return nil
		}
		if execErr == nil {
			if result == nil {
				result = &StepResult{}
			}
			result.Attempts = attempt
			err := e.completeStep(ctx, caseID, step.ID, result)
			e.observeStepExecution(ctx, caseID, step.Type, start)
			return err
		}

		retryCount, updateErr := e.incrementRetryCount(ctx, caseID, step.ID, attempt, execErr)
		if updateErr != nil {
			return updateErr
		}
		if retryCount < policy.MaxAttempts {
			slog.WarnContext(ctx, "step retry scheduled",
				append(observability.RequestAttrs(ctx),
					"case_id", caseID.String(),
					"step_id", step.ID,
					"step_type", step.Type,
					"retry_count", retryCount,
					"max_attempts", policy.MaxAttempts,
				)...,
			)
			time.Sleep(calculateBackoff(policy, retryCount))
			continue
		}
		err := e.onExhausted(ctx, caseID, step, attempt, execErr)
		e.observeStepExecution(ctx, caseID, step.Type, start)
		return err
	}
}

func (e *Engine) observeStepExecution(ctx context.Context, caseID uuid.UUID, stepType string, start time.Time) {
	tenantID, err := e.lookupTenantID(ctx, caseID)
	if err != nil {
		return
	}
	observability.StepExecutionDurationSeconds.WithLabelValues(tenantID.String(), stepType).Observe(time.Since(start).Seconds())
}

func (e *Engine) lookupTenantID(ctx context.Context, caseID uuid.UUID) (uuid.UUID, error) {
	var tenantID uuid.UUID
	if err := e.db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1`, caseID).Scan(&tenantID); err != nil {
		return uuid.Nil, err
	}
	return tenantID, nil
}

func (e *Engine) incrementRetryCount(ctx context.Context, caseID uuid.UUID, stepID string, attempt int, execErr error) (int, error) {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin retry update tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var retryCount int
	err = tx.QueryRowContext(ctx, `
	UPDATE case_steps
	SET
	    retry_count = retry_count + 1,
	    events = COALESCE(events, '[]'::jsonb) || jsonb_build_array(
	        jsonb_build_object('type', 'retry_attempt', 'attempt', $3::int, 'error', $4::text, 'at', now())
	    )
WHERE case_id = $1 AND step_id = $2 AND state = 'active'
RETURNING retry_count
`, caseID, stepID, attempt, execErr.Error()).Scan(&retryCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("step %s is no longer active", stepID)
		}
		return 0, fmt.Errorf("update retry count: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return 0, fmt.Errorf("touch case during retry update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit retry update: %w", err)
	}
	return retryCount, nil
}

func (e *Engine) onExhausted(ctx context.Context, caseID uuid.UUID, step WorkflowStep, attempts int, execErr error) error {
	policy := defaultErrorPolicyForStep(step.Type, step.ErrorPolicy)
	action := policy.OnExhausted

	switch {
	case action == "skip":
		if err := e.skipStepTerminal(ctx, caseID, step.ID, attempts, execErr); err != nil {
			return err
		}
		e.triggerEvaluation(caseID)
		return nil
	case strings.HasPrefix(action, "goto:"):
		target := strings.TrimPrefix(action, "goto:")
		if err := e.failStep(ctx, caseID, step.ID, execErr); err != nil {
			return err
		}
		if err := e.activateFallbackStep(ctx, caseID, target); err != nil {
			return err
		}
		e.triggerEvaluation(caseID)
		return nil
	default:
		return e.failStep(ctx, caseID, step.ID, execErr)
	}
}

func (e *Engine) activateFallbackStep(ctx context.Context, caseID uuid.UUID, stepID string) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fallback activation tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET state = 'ready'
WHERE case_id = $1 AND step_id = $2 AND state = 'pending'
`, caseID, stepID); err != nil {
		return fmt.Errorf("activate fallback step %s: %w", stepID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case fallback activation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fallback activation: %w", err)
	}
	return nil
}

func calculateBackoff(policy ErrorPolicy, retryCount int) time.Duration {
	policy = defaultErrorPolicyForStep("", policy)
	delay := policy.InitialDelay
	switch policy.Backoff {
	case "none":
		delay = 0
	case "linear":
		delay = time.Duration(retryCount) * policy.InitialDelay
	case "exponential":
		delay = policy.InitialDelay << (retryCount - 1)
	}
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	if delay < 0 {
		return policy.InitialDelay
	}
	return delay
}
