package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/notify"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (s *TaskService) EscalateTask(ctx context.Context, tenantID uuid.UUID, caseID uuid.UUID, stepID string, cfg EscalationConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin escalate task tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()

	var state string
	var currentAssigned sql.NullString
	if err := tx.QueryRowContext(ctx, `
SELECT cs.state, cs.assigned_to
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE cs.case_id=$1 AND cs.step_id=$2 AND c.tenant_id=$3
FOR UPDATE
`, caseID, stepID, tenantID).Scan(&state, &currentAssigned); err != nil {
		return err
	}
	if state != engine.StateActive {
		if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", s.systemActor(), "system", "escalation_suppressed", map[string]any{"state": state}); err != nil {
			return err
		}
		return s.auditSvc.CommitTx(tx)
	}

	var reassignedTo *uuid.UUID
	if cfg.Action == "reassign" || cfg.Action == "both" {
		uid, err := s.pickLeastLoadedUserTx(ctx, tx, tenantID, cfg.ToRole)
		if err == nil && uid != uuid.Nil {
			reassignedTo = &uid
			if _, err := tx.ExecContext(ctx, `UPDATE case_steps SET assigned_to=$3 WHERE case_id=$1 AND step_id=$2`, caseID, stepID, uid); err != nil {
				return fmt.Errorf("escalation reassign: %w", err)
			}
		}
	}

	if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", s.systemActor(), "system", "escalated", map[string]any{"to_role": cfg.ToRole, "action": cfg.Action, "assigned_to": reassignedTo}); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id=$1`, caseID); err != nil {
		return fmt.Errorf("touch case escalate: %w", err)
	}
	if err := s.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit escalate tx: %w", err)
	}

	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, tenantID, caseID)
		recipients := make([]notify.Recipient, 0)
		if cfg.ToRole != "" {
			roleRecipients, _ := s.resolveRoleRecipients(ctx, tenantID, cfg.ToRole, []string{"email", "websocket"})
			recipients = append(recipients, roleRecipients...)
		}
		if reassignedTo != nil {
			email, _ := s.lookupPrincipalEmail(ctx, *reassignedTo)
			recipients = append(recipients, notify.Recipient{PrincipalID: *reassignedTo, Email: email, Channels: []string{"email", "websocket"}})
		}
		_ = s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "task_escalated",
			TenantID:   tenantID,
			CaseID:     caseID,
			CaseNumber: caseNumber,
			StepID:     stepID,
			StepLabel:  stepID,
			Recipients: dedupeRecipients(recipients),
			Data:       map[string]any{"to_role": cfg.ToRole, "action": cfg.Action},
		})
	}
	return nil
}

func (s *TaskService) HandleOverdue(ctx context.Context, task engine.OverdueTask) error {
	cfg, tenantID, err := s.loadEscalationConfig(ctx, task.CaseID, task.StepID)
	if err != nil {
		return err
	}
	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, tenantID, task.CaseID)
		recipients := make([]notify.Recipient, 0)
		if task.AssignedTo != nil {
			email, _ := s.lookupPrincipalEmail(ctx, *task.AssignedTo)
			recipients = append(recipients, notify.Recipient{PrincipalID: *task.AssignedTo, Email: email, Channels: []string{"email", "websocket"}})
		}
		if cfg.ToRole != "" {
			roleRecipients, _ := s.resolveRoleRecipients(ctx, tenantID, cfg.ToRole, []string{"email", "websocket"})
			recipients = append(recipients, roleRecipients...)
		}
		_ = s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "sla_breach",
			TenantID:   tenantID,
			CaseID:     task.CaseID,
			CaseNumber: caseNumber,
			StepID:     task.StepID,
			StepLabel:  task.StepID,
			Recipients: dedupeRecipients(recipients),
			Data:       map[string]any{"sla_deadline": task.SLADeadline.UTC().Format(time.RFC3339Nano)},
		})
	}
	if cfg.Action == "" {
		return nil
	}
	return s.EscalateTask(ctx, tenantID, task.CaseID, task.StepID, cfg)
}

func (s *TaskService) updateActiveTasksGauge(ctx context.Context, tenantID uuid.UUID) {
	if tenantID == uuid.Nil {
		return
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE c.tenant_id = $1
  AND cs.state = 'active'
`, tenantID).Scan(&count); err == nil {
		observability.TasksActiveTotal.WithLabelValues(tenantID.String()).Set(float64(count))
	}
}

func (s *TaskService) loadEscalationConfig(ctx context.Context, caseID uuid.UUID, stepID string) (EscalationConfig, uuid.UUID, error) {
	step, err := s.loadWorkflowStep(ctx, caseID, stepID)
	if err != nil {
		return EscalationConfig{}, uuid.Nil, err
	}
	cfg := AssignmentConfig{}
	_ = json.Unmarshal(step.Config, &cfg)
	var tenantID uuid.UUID
	if err := s.db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id=$1`, caseID).Scan(&tenantID); err != nil {
		return EscalationConfig{}, uuid.Nil, err
	}
	return cfg.Escalation, tenantID, nil
}

func (s *TaskService) pickLeastLoadedUserTx(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, role string) (uuid.UUID, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT p.id
     , COUNT(cs.id) AS active_count
     , p.created_at
FROM principals p
JOIN principal_roles pr ON pr.principal_id = p.id
JOIN roles r ON r.id = pr.role_id
LEFT JOIN case_steps cs ON cs.assigned_to = p.id AND cs.state='active'
WHERE p.tenant_id=$1 AND p.status='active' AND r.name=$2
GROUP BY p.id
`, tenantID, role)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = rows.Close() }()
	candidates := make([]userLoad, 0)
	for rows.Next() {
		var c userLoad
		if err := rows.Scan(&c.PrincipalID, &c.ActiveCount, &c.CreatedAt); err != nil {
			return uuid.Nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, err
	}
	selected := selectLeastLoaded(candidates)
	if selected == uuid.Nil {
		return uuid.Nil, sql.ErrNoRows
	}
	return selected, nil
}

func selectLeastLoaded(candidates []userLoad) uuid.UUID {
	if len(candidates) == 0 {
		return uuid.Nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ActiveCount != candidates[j].ActiveCount {
			return candidates[i].ActiveCount < candidates[j].ActiveCount
		}
		if !candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
		}
		return candidates[i].PrincipalID.String() < candidates[j].PrincipalID.String()
	})
	return candidates[0].PrincipalID
}
