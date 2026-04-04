package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (e *Engine) EvaluateDAG(ctx context.Context, caseID uuid.UUID) error {
	return e.evaluateDAG(ctx, caseID)
}

func (e *Engine) evaluateDAG(ctx context.Context, caseID uuid.UUID) error {
	start := time.Now()
	defer func() {
		observability.DBQueryDurationSeconds.WithLabelValues("dag_eval").Observe(time.Since(start).Seconds())
	}()
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dag evaluation tx: %w", err)
	}
	defer func() { _ = e.auditSvc.RollbackTx(tx) }()

	var caseStatus string
	var caseData []byte
	var tenantID uuid.UUID
	err = tx.QueryRowContext(ctx, `
SELECT status, data, tenant_id
FROM cases
WHERE id = $1
FOR UPDATE
`, caseID).Scan(&caseStatus, &caseData, &tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock case row: %w", err)
	}
	if caseStatus == "cancelled" {
		observability.DAGEvaluationsTotal.WithLabelValues(tenantID.String()).Inc()
		observability.DAGEvaluationDurationSeconds.WithLabelValues(tenantID.String()).Observe(time.Since(start).Seconds())
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
		switch tr.Type {
		case TransitionToActive:
			if err := e.auditSvc.RecordCaseEventTx(ctx, tx, caseID, tr.StepID, "step", e.systemActor(), "system", "activated", map[string]any{"from": tr.From, "to": tr.To, "reason": tr.Reason, "outcome": tr.Outcome}); err != nil {
				return err
			}
		case TransitionToSkipped:
			if err := e.auditSvc.RecordCaseEventTx(ctx, tx, caseID, tr.StepID, "step", e.systemActor(), "system", "skipped", map[string]any{"from": tr.From, "to": tr.To, "reason": tr.Reason, "outcome": tr.Outcome}); err != nil {
				return err
			}
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

	if err := e.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit dag evaluation: %w", err)
	}
	e.updateCaseStepStateMetrics(ctx, tenantID)
	observability.DAGEvaluationsTotal.WithLabelValues(tenantID.String()).Inc()
	observability.DAGEvaluationDurationSeconds.WithLabelValues(tenantID.String()).Observe(time.Since(start).Seconds())
	active, capTotal := e.WorkerPoolStats()
	if capTotal > 0 {
		observability.WorkerPoolUtilisation.Set(float64(active) / float64(capTotal))
	}
	slog.DebugContext(ctx, "dag evaluation completed",
		append(observability.RequestAttrs(ctx),
			"case_id", caseID.String(),
			"tenant_id", tenantID.String(),
			"transitions", len(transitions),
			"duration_ms", time.Since(start).Milliseconds(),
		)...,
	)

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
	defer func() { _ = rows.Close() }()

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
	if step.Type == "human_task" {
		_ = e.executeWithRetry(context.Background(), caseID, step)
		return
	}
	e.executions.Submit(func() {
		_ = e.executeWithRetry(context.Background(), caseID, step)
	})
}

func (e *Engine) systemActor() uuid.UUID {
	e.mu.RLock()
	actorID := e.systemActorID
	e.mu.RUnlock()
	if actorID != uuid.Nil {
		return actorID
	}
	return uuid.MustParse("00000000-0000-0000-0000-000000000000")
}
