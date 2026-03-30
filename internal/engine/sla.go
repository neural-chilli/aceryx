package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (e *Engine) StartSLAMonitor(ctx context.Context) {
	ticker := time.NewTicker(e.slaInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for {
				count, err := e.checkOverdueTasks(ctx)
				if err != nil {
					break
				}
				if count < 1000 {
					break
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) checkOverdueTasks(ctx context.Context) (int, error) {
	e.mu.RLock()
	cb := e.escalation
	e.mu.RUnlock()
	if cb == nil {
		return 0, nil
	}

	rows, err := e.db.QueryContext(ctx, `
SELECT cs.id, cs.case_id, cs.step_id, cs.sla_deadline, cs.assigned_to
FROM case_steps cs
WHERE cs.state = 'active'
  AND cs.sla_deadline IS NOT NULL
  AND cs.sla_deadline < now()
ORDER BY cs.sla_deadline
LIMIT 1000
`)
	if err != nil {
		return 0, fmt.Errorf("query overdue tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tasks := make([]OverdueTask, 0)
	for rows.Next() {
		var (
			task       OverdueTask
			assignedTo sql.NullString
		)
		if err := rows.Scan(&task.ID, &task.CaseID, &task.StepID, &task.SLADeadline, &assignedTo); err != nil {
			return 0, fmt.Errorf("scan overdue task: %w", err)
		}
		if assignedTo.Valid {
			id, parseErr := uuid.Parse(assignedTo.String)
			if parseErr == nil {
				task.AssignedTo = &id
			}
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate overdue tasks: %w", err)
	}

	for _, task := range tasks {
		var state string
		if err := e.db.QueryRowContext(ctx, `SELECT state FROM case_steps WHERE id = $1`, task.ID).Scan(&state); err != nil {
			continue
		}
		if state != StateActive {
			continue
		}
		_ = e.recordSLABreach(ctx, task)
		_ = cb(ctx, task)
		tenantID, terr := e.lookupTenantID(ctx, task.CaseID)
		if terr == nil {
			observability.SLABreachesTotal.WithLabelValues(tenantID.String()).Inc()
		}
		slog.WarnContext(ctx, "sla breach detected",
			append(observability.RequestAttrs(ctx),
				"case_id", task.CaseID.String(),
				"step_id", task.StepID,
				"task_id", task.ID.String(),
			)...,
		)
	}

	return len(tasks), nil
}

// CheckOverdueTasksForTest exposes one SLA scan pass for integration tests.
func (e *Engine) CheckOverdueTasksForTest(ctx context.Context) (int, error) {
	return e.checkOverdueTasks(ctx)
}

func (e *Engine) recordSLABreach(ctx context.Context, task OverdueTask) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = audit.RollbackTx(tx) }()

	var caseStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM cases WHERE id = $1 FOR UPDATE`, task.CaseID).Scan(&caseStatus); err != nil {
		return err
	}
	if caseStatus == "cancelled" {
		return audit.CommitTx(tx)
	}

	var state string
	var deadline sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT state, sla_deadline
FROM case_steps
WHERE id = $1 AND case_id = $2 AND step_id = $3
FOR UPDATE
`, task.ID, task.CaseID, task.StepID).Scan(&state, &deadline); err != nil {
		return err
	}
	if state != StateActive || !deadline.Valid || !deadline.Time.Before(time.Now().UTC()) {
		return audit.CommitTx(tx)
	}

	if err := audit.RecordCaseEventTx(ctx, tx, task.CaseID, task.StepID, "system", e.systemActor(), "system", "sla_breach", map[string]any{
		"sla_deadline": deadline.Time.UTC().Format(time.RFC3339Nano),
		"task_id":      task.ID.String(),
	}); err != nil {
		return err
	}
	return audit.CommitTx(tx)
}
