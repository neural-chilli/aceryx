package engine

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	defer rows.Close()

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
		_ = cb(ctx, task)
	}

	return len(tasks), nil
}

// CheckOverdueTasksForTest exposes one SLA scan pass for integration tests.
func (e *Engine) CheckOverdueTasksForTest(ctx context.Context) (int, error) {
	return e.checkOverdueTasks(ctx)
}
