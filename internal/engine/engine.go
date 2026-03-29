package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (e *Engine) EvaluateDAG(ctx context.Context, caseID uuid.UUID) error {
	return e.evaluateDAG(ctx, caseID)
}

func (e *Engine) evaluateDAG(ctx context.Context, caseID uuid.UUID) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dag evaluation tx: %w", err)
	}
	defer tx.Rollback()

	var caseStatus string
	var caseData []byte
	err = tx.QueryRowContext(ctx, `
SELECT status, data
FROM cases
WHERE id = $1
FOR UPDATE
`, caseID).Scan(&caseStatus, &caseData)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock case row: %w", err)
	}
	if caseStatus == "cancelled" {
		return tx.Commit()
	}

	states, err := loadStepStatesTx(ctx, tx, caseID)
	if err != nil {
		return err
	}

	ast, err := loadWorkflowASTTx(ctx, tx, caseID)
	if err != nil {
		return err
	}

	evalContext := map[string]interface{}{"case": map[string]interface{}{}}
	if len(caseData) > 0 {
		var caseObj map[string]interface{}
		if err := json.Unmarshal(caseData, &caseObj); err == nil {
			evalContext["case"] = caseObj
		}
	}

	transitions, err := computeTransitions(ast, states, e.evaluators, evalContext)
	if err != nil {
		return fmt.Errorf("compute transitions: %w", err)
	}

	stepsByID := stepMap(ast)
	toDispatch := make([]WorkflowStep, 0)
	for _, tr := range transitions {
		if err := applyTransitionTx(ctx, tx, caseID, tr); err != nil {
			return err
		}
		if tr.Type == TransitionToActive {
			if step, ok := stepsByID[tr.StepID]; ok {
				toDispatch = append(toDispatch, step)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("update case timestamp after dag evaluation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dag evaluation: %w", err)
	}

	for _, step := range toDispatch {
		e.dispatchStep(caseID, step)
	}
	return nil
}

func applyTransitionTx(ctx context.Context, tx *sql.Tx, caseID uuid.UUID, tr Transition) error {
	switch tr.Type {
	case TransitionToReady:
		if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET state = 'ready'
WHERE case_id = $1 AND step_id = $2 AND state = $3
`, caseID, tr.StepID, tr.From); err != nil {
			return fmt.Errorf("apply ready transition for %s: %w", tr.StepID, err)
		}
	case TransitionToSkipped:
		if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET state = 'skipped', completed_at = now()
WHERE case_id = $1 AND step_id = $2 AND state IN ($3, $4)
`, caseID, tr.StepID, StatePending, StateReady); err != nil {
			return fmt.Errorf("apply skipped transition for %s: %w", tr.StepID, err)
		}
	case TransitionToActive:
		if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET
    state = 'active',
    started_at = COALESCE(started_at, now()),
    metadata = CASE
        WHEN metadata IS NULL THEN jsonb_build_object('request_id', gen_random_uuid()::text)
        WHEN metadata ? 'request_id' THEN metadata
        ELSE jsonb_set(metadata, '{request_id}', to_jsonb(gen_random_uuid()::text), true)
    END
WHERE case_id = $1 AND step_id = $2 AND state = $3
`, caseID, tr.StepID, StateReady); err != nil {
			return fmt.Errorf("apply active transition for %s: %w", tr.StepID, err)
		}
	default:
		return fmt.Errorf("unsupported transition type %q", tr.Type)
	}
	return nil
}

func loadStepStatesTx(ctx context.Context, tx *sql.Tx, caseID uuid.UUID) (map[string]StepState, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT step_id, state, result, retry_count, metadata, completed_at
FROM case_steps
WHERE case_id = $1
`, caseID)
	if err != nil {
		return nil, fmt.Errorf("load step states: %w", err)
	}
	defer rows.Close()

	states := make(map[string]StepState)
	for rows.Next() {
		var st StepState
		var resultRaw []byte
		var metadata []byte
		var completed sql.NullTime
		if err := rows.Scan(&st.StepID, &st.State, &resultRaw, &st.RetryCount, &metadata, &completed); err != nil {
			return nil, fmt.Errorf("scan step state: %w", err)
		}
		if len(resultRaw) > 0 {
			st.Result = resultRaw
		}
		if len(metadata) > 0 {
			st.Metadata = metadata
		}
		if completed.Valid {
			t := completed.Time
			st.CompletedAt = &t
		}
		states[st.StepID] = st
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate step states: %w", err)
	}
	return states, nil
}

func loadWorkflowASTTx(ctx context.Context, tx *sql.Tx, caseID uuid.UUID) (WorkflowAST, error) {
	var raw []byte
	err := tx.QueryRowContext(ctx, `
SELECT wv.ast
FROM cases c
JOIN workflow_versions wv ON wv.workflow_id = c.workflow_id AND wv.version = c.workflow_version
WHERE c.id = $1
`, caseID).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkflowAST{}, ErrNotFound
		}
		return WorkflowAST{}, fmt.Errorf("load workflow ast: %w", err)
	}
	ast, err := parseAST(raw)
	if err != nil {
		return WorkflowAST{}, err
	}
	return ast, nil
}

func (e *Engine) triggerEvaluation(caseID uuid.UUID) {
	e.evaluations.Submit(func() {
		_ = e.evaluateDAG(context.Background(), caseID)
	})
}

func (e *Engine) dispatchStep(caseID uuid.UUID, step WorkflowStep) {
	e.executions.Submit(func() {
		_ = e.executeWithRetry(context.Background(), caseID, step)
	})
}

func (e *Engine) executeWithRetry(ctx context.Context, caseID uuid.UUID, step WorkflowStep) error {
	exec, err := e.executorFor(step.Type)
	if err != nil {
		return err
	}
	policy := defaultErrorPolicyForStep(step.Type, step.ErrorPolicy)

	attempt := 0
	for {
		attempt++
		result, execErr := exec.Execute(ctx, caseID, step.ID, step.Config)
		if execErr == nil {
			if result == nil {
				result = &StepResult{}
			}
			result.Attempts = attempt
			return e.completeStep(ctx, caseID, step.ID, result)
		}

		retryCount, updateErr := e.incrementRetryCount(ctx, caseID, step.ID, attempt, execErr)
		if updateErr != nil {
			return updateErr
		}
		if retryCount < policy.MaxAttempts {
			time.Sleep(calculateBackoff(policy, retryCount))
			continue
		}
		return e.onExhausted(ctx, caseID, step, attempt, execErr)
	}
}

func (e *Engine) incrementRetryCount(ctx context.Context, caseID uuid.UUID, stepID string, attempt int, execErr error) (int, error) {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin retry update tx: %w", err)
	}
	defer tx.Rollback()

	var retryCount int
	err = tx.QueryRowContext(ctx, `
UPDATE case_steps
SET
    retry_count = retry_count + 1,
    events = COALESCE(events, '[]'::jsonb) || jsonb_build_array(
        jsonb_build_object('type', 'retry_attempt', 'attempt', $3, 'error', $4, 'at', now())
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
	defer tx.Rollback()

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
	defer tx.Rollback()

	var caseStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM cases WHERE id = $1 FOR UPDATE`, caseID).Scan(&caseStatus); err != nil {
		return fmt.Errorf("lock case for complete step: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal step result: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET
    state = 'completed',
    completed_at = now(),
    result = $3::jsonb,
    error = NULL,
    events = COALESCE(events, '[]'::jsonb) || jsonb_build_array(
        jsonb_build_object('type', 'completed', 'attempts', $4, 'at', now())
    )
WHERE case_id = $1 AND step_id = $2 AND state = 'active'
`, caseID, stepID, string(resultJSON), result.Attempts); err != nil {
		return fmt.Errorf("update completed step state: %w", err)
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

	if err := e.insertCaseEventTx(ctx, tx, caseID, stepID, "step_completed", "system", "complete_step", map[string]interface{}{"attempts": result.Attempts, "outcome": result.Outcome}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit complete step: %w", err)
	}

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
	defer tx.Rollback()

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
	if err := e.insertCaseEventTx(ctx, tx, caseID, stepID, "step_failed", "system", "fail_step", map[string]interface{}{"error": failErr.Error()}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fail step: %w", err)
	}

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
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET
    state = 'skipped',
    completed_at = now(),
    error = jsonb_build_object('message', $3),
    events = COALESCE(events, '[]'::jsonb) || jsonb_build_array(
        jsonb_build_object('type', 'skipped_on_exhausted', 'attempts', $4, 'error', $3, 'at', now())
    )
WHERE case_id = $1 AND step_id = $2 AND state = 'active'
`, caseID, stepID, cause.Error(), attempts); err != nil {
		return fmt.Errorf("mark step skipped on exhausted policy: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case for skip-on-exhausted: %w", err)
	}
	if err := e.insertCaseEventTx(ctx, tx, caseID, stepID, "step_skipped", "system", "skip_step", map[string]interface{}{"attempts": attempts, "error": cause.Error()}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit skip step terminal: %w", err)
	}
	return nil
}

func (e *Engine) CancelCase(ctx context.Context, caseID uuid.UUID, actorID uuid.UUID, reason string) error {
	return e.cancelCase(ctx, caseID, actorID, reason)
}

func (e *Engine) cancelCase(ctx context.Context, caseID uuid.UUID, actorID uuid.UUID, reason string) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cancel case tx: %w", err)
	}
	defer tx.Rollback()

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

	if err := e.insertCaseEventTx(ctx, tx, caseID, "", "case_cancelled", "human", "cancel_case", map[string]interface{}{"reason": reason, "actor_id": actorID.String()}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cancel case: %w", err)
	}
	return nil
}

func (e *Engine) insertCaseEventTx(ctx context.Context, tx *sql.Tx, caseID uuid.UUID, stepID, eventType, actorType, action string, data map[string]interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal case event payload: %w", err)
	}
	actorID := e.systemActorID
	if actorID == uuid.Nil {
		actorID = uuid.MustParse("00000000-0000-0000-0000-000000000000")
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO case_events (case_id, step_id, event_type, actor_id, actor_type, action, data, prev_event_hash, event_hash)
VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7::jsonb, $8, $9)
`, caseID, stepID, eventType, actorID, actorType, action, string(payload), "prev", "event"); err != nil {
		return fmt.Errorf("insert case event %s: %w", eventType, err)
	}
	return nil
}

func calculateBackoff(policy ErrorPolicy, retryCount int) time.Duration {
	policy = defaultErrorPolicyForStep("", policy)
	delay := policy.InitialDelay
	switch policy.Backoff {
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
