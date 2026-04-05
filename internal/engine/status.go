package engine

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

func (e *Engine) GetStatus(ctx context.Context, tenantID, caseID uuid.UUID) (WorkflowStatus, error) {
	if e == nil || e.db == nil {
		return WorkflowStatus{}, fmt.Errorf("engine not configured")
	}
	var status WorkflowStatus
	status.CaseID = caseID
	if err := e.db.QueryRowContext(ctx, `
SELECT c.status, c.workflow_id
FROM cases c
WHERE c.tenant_id = $1 AND c.id = $2
`, tenantID, caseID).Scan(&status.Status, &status.WorkflowID); err != nil {
		if err == sql.ErrNoRows {
			return WorkflowStatus{}, ErrNotFound
		}
		return WorkflowStatus{}, fmt.Errorf("load workflow status: %w", err)
	}

	rows, err := e.db.QueryContext(ctx, `
SELECT step_id, state
FROM case_steps
WHERE case_id = $1
`, caseID)
	if err != nil {
		return WorkflowStatus{}, fmt.Errorf("load case steps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	total := 0
	completed := 0
	pendingTasks := 0
	for rows.Next() {
		var stepID string
		var state string
		if err := rows.Scan(&stepID, &state); err != nil {
			return WorkflowStatus{}, fmt.Errorf("scan case step state: %w", err)
		}
		total++
		switch state {
		case StateCompleted, StateSkipped:
			completed++
		case StateActive:
			pendingTasks++
			if status.CurrentStep == "" {
				status.CurrentStep = stepID
			}
		}
	}
	if err := rows.Err(); err != nil {
		return WorkflowStatus{}, fmt.Errorf("iterate case step states: %w", err)
	}

	status.CompletedSteps = completed
	status.PendingTasks = pendingTasks
	if total > 0 {
		status.ProgressPct = float64(completed) * 100 / float64(total)
	}
	return status, nil
}
