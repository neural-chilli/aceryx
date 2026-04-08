package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/notify"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (s *TaskService) ClaimTask(ctx context.Context, tenantID, principalID, caseID uuid.UUID, stepID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin claim task tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()

	var claimed bool
	err = tx.QueryRowContext(ctx, `
WITH claim AS (
    UPDATE case_steps cs
    SET assigned_to = $4
    FROM cases c
WHERE cs.case_id = $1
      AND cs.step_id = $2
      AND cs.case_id = c.id
      AND c.tenant_id = $3
      AND cs.state = 'active'
      AND cs.assigned_to IS NULL
      AND (
          COALESCE(cs.metadata->>'role', '') = ''
          OR EXISTS (
              SELECT 1
              FROM principal_roles pr
              JOIN roles r ON r.id = pr.role_id
              WHERE pr.principal_id = $4
                AND r.tenant_id = $3
                AND r.name = COALESCE(cs.metadata->>'role', '')
          )
      )
    RETURNING 1
)
SELECT EXISTS(SELECT 1 FROM claim)
`, caseID, stepID, tenantID, principalID).Scan(&claimed)
	if err != nil {
		return fmt.Errorf("claim task: %w", err)
	}
	if !claimed {
		var (
			assignedTo   sql.NullString
			requiredRole string
		)
		rowErr := tx.QueryRowContext(ctx, `
SELECT cs.assigned_to, COALESCE(cs.metadata->>'role', '')
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE cs.case_id = $1
  AND cs.step_id = $2
  AND c.tenant_id = $3
  AND cs.state = 'active'
`, caseID, stepID, tenantID).Scan(&assignedTo, &requiredRole)
		if rowErr != nil {
			return ErrAlreadyClaimed
		}
		if assignedTo.Valid {
			return ErrAlreadyClaimed
		}
		if requiredRole != "" {
			var eligible bool
			eligibilityErr := tx.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM principal_roles pr
    JOIN roles r ON r.id = pr.role_id
    WHERE pr.principal_id = $1
      AND r.tenant_id = $2
      AND r.name = $3
)
`, principalID, tenantID, requiredRole).Scan(&eligible)
			if eligibilityErr == nil && !eligible {
				return ErrForbidden
			}
		}
		return ErrAlreadyClaimed
	}

	if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", principalID, "human", "claimed", map[string]any{}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case task claim: %w", err)
	}
	if err := s.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit claim task: %w", err)
	}

	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, tenantID, caseID)
		roleName, _ := s.lookupStepRole(ctx, tenantID, caseID, stepID)
		recipients := make([]notify.Recipient, 0)
		if roleName != "" {
			recipients, _ = s.resolveRoleRecipients(ctx, tenantID, roleName, []string{"websocket"})
		}
		if err := s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "task_claimed",
			TenantID:   tenantID,
			CaseID:     caseID,
			CaseNumber: caseNumber,
			StepID:     stepID,
			StepLabel:  stepID,
			Recipients: recipients,
			Data:       map[string]any{"claimed_by": principalID.String()},
		}); err != nil {
			slog.WarnContext(ctx, "task claim notification failed", "tenant_id", tenantID.String(), "case_id", caseID.String(), "step_id", stepID, "error", err)
		}
	}
	observability.TasksClaimedTotal.WithLabelValues(tenantID.String()).Inc()
	s.updateActiveTasksGauge(ctx, tenantID)
	slog.InfoContext(ctx, "task claimed",
		append(observability.RequestAttrs(ctx),
			"tenant_id", tenantID.String(),
			"principal_id", principalID.String(),
			"case_id", caseID.String(),
			"step_id", stepID,
		)...,
	)
	return nil
}

func (s *TaskService) SaveDraft(ctx context.Context, tenantID, principalID, caseID uuid.UUID, stepID string, req DraftRequest) error {
	dataRaw, err := json.Marshal(req.Data)
	if err != nil {
		return fmt.Errorf("marshal draft data: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE case_steps cs
SET draft_data = $5::jsonb
FROM cases c
WHERE cs.case_id = $1 AND cs.step_id = $2 AND c.id = cs.case_id AND c.tenant_id = $3 AND cs.state='active' AND cs.assigned_to = $4
`, caseID, stepID, tenantID, principalID, string(dataRaw))
	if err != nil {
		return fmt.Errorf("save draft: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrForbidden
	}
	return nil
}

func (s *TaskService) CompleteTask(ctx context.Context, tenantID, principalID, caseID uuid.UUID, stepID string, req CompleteTaskRequest) error {
	var (
		taskDetail TaskDetail
		err        error
	)
	deadline := time.Now().Add(2 * time.Second)
	for {
		taskDetail, err = s.GetTask(ctx, tenantID, caseID, stepID)
		if err != nil {
			return err
		}
		if taskDetail.StepState == engine.StateCompleted {
			return ErrAlreadyCompleted
		}
		assigned := taskDetail.AssignedTo != nil && *taskDetail.AssignedTo == principalID
		outcomeAllowed := contains(taskDetail.Outcomes, req.Outcome)
		if assigned && outcomeAllowed {
			break
		}
		waitForAgentInit := taskDetail.StepType == "agent" && taskDetail.StepState == engine.StateActive && time.Now().Before(deadline)
		if !waitForAgentInit {
			if !assigned {
				return ErrForbidden
			}
			return ErrInvalidOutcome
		}
		time.Sleep(50 * time.Millisecond)
	}
	payload := req.Data
	if taskDetail.Form == "agent_review" && strings.EqualFold(req.Outcome, "accept") && len(payload) == 0 {
		payload = extractAgentOriginalOutput(taskDetail.Metadata)
	}
	validation := ValidateFormData(taskDetail.FormSchema, payload)
	validation = append(validation, ValidateActionRequirements(taskDetail.FormSchema, req.Outcome, payload)...)
	if len(validation) > 0 {
		return fmt.Errorf("validation_failed: %s", validation[0].Message)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin complete task tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()

	resultRaw, err := json.Marshal(map[string]any{"outcome": req.Outcome, "data": payload})
	if err != nil {
		return fmt.Errorf("marshal task completion result: %w", err)
	}
	res, err := tx.ExecContext(ctx, `
UPDATE case_steps cs
SET state='completed', completed_at=now(), result=$5::jsonb, draft_data=NULL,
    events = COALESCE(events, '[]'::jsonb) || jsonb_build_array(jsonb_build_object('type','completed','outcome',$6::text,'at',now()))
FROM cases c
WHERE cs.case_id=$1 AND cs.step_id=$2 AND c.id=cs.case_id AND c.tenant_id=$3 AND cs.state='active' AND cs.assigned_to=$4
`, caseID, stepID, tenantID, principalID, string(resultRaw), req.Outcome)
	if err != nil {
		return fmt.Errorf("update task completion: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		var existingState string
		_ = tx.QueryRowContext(ctx, `SELECT state FROM case_steps WHERE case_id=$1 AND step_id=$2`, caseID, stepID).Scan(&existingState)
		if existingState == engine.StateCompleted {
			return ErrAlreadyCompleted
		}
		return ErrForbidden
	}

	decisionPatch := buildDecisionPatch(taskDetail.FormSchema, payload)
	if len(decisionPatch) > 0 {
		patchRaw, err := json.Marshal(decisionPatch)
		if err != nil {
			return fmt.Errorf("marshal decision patch: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE cases
SET data = COALESCE(data, '{}'::jsonb) || jsonb_build_object(
    'decision',
    (
      CASE
        WHEN jsonb_typeof(data->'decision') = 'object' THEN data->'decision'
        ELSE '{}'::jsonb
      END
    ) || $2::jsonb
),
    version = version + 1,
    updated_at = now()
WHERE id = $1
`, caseID, string(patchRaw)); err != nil {
			return fmt.Errorf("update case decision data from task: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
			return fmt.Errorf("touch case completion: %w", err)
		}
	}

	if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", principalID, "human", "completed", map[string]any{"outcome": req.Outcome, "data": payload}); err != nil {
		return err
	}

	if err := s.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit complete task tx: %w", err)
	}

	if s.engine != nil {
		_ = s.engine.EvaluateDAG(ctx, caseID)
	}
	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, tenantID, caseID)
		caseAssignee, _ := s.lookupCaseAssignee(ctx, tenantID, caseID)
		recipients := make([]notify.Recipient, 0)
		if caseAssignee != nil && *caseAssignee != principalID {
			email, _ := s.lookupPrincipalEmail(ctx, *caseAssignee)
			recipients = append(recipients, notify.Recipient{PrincipalID: *caseAssignee, Email: email, Channels: []string{"websocket"}})
		}
		if err := s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "task_completed",
			TenantID:   tenantID,
			CaseID:     caseID,
			CaseNumber: caseNumber,
			StepID:     stepID,
			StepLabel:  stepID,
			Recipients: recipients,
			Data:       map[string]any{"outcome": req.Outcome, "completed_by": principalID.String()},
		}); err != nil {
			slog.WarnContext(ctx, "task completion notification failed", "tenant_id", tenantID.String(), "case_id", caseID.String(), "step_id", stepID, "error", err)
		}
	}
	observability.TasksCompletedTotal.WithLabelValues(tenantID.String(), req.Outcome).Inc()
	s.updateActiveTasksGauge(ctx, tenantID)
	slog.InfoContext(ctx, "task completed",
		append(observability.RequestAttrs(ctx),
			"tenant_id", tenantID.String(),
			"principal_id", principalID.String(),
			"case_id", caseID.String(),
			"step_id", stepID,
			"outcome", req.Outcome,
		)...,
	)
	return nil
}

func (s *TaskService) ReassignTask(ctx context.Context, tenantID, actorID, caseID uuid.UUID, stepID string, req ReassignRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reassign task tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()

	var oldAssigned sql.NullString
	if err := tx.QueryRowContext(ctx, `
SELECT cs.assigned_to
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE cs.case_id=$1 AND cs.step_id=$2 AND c.tenant_id=$3 AND cs.state='active'
FOR UPDATE
`, caseID, stepID, tenantID).Scan(&oldAssigned); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE case_steps SET assigned_to=$3 WHERE case_id=$1 AND step_id=$2`, caseID, stepID, req.AssignTo); err != nil {
		return fmt.Errorf("reassign task: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id=$1`, caseID); err != nil {
		return fmt.Errorf("touch case reassign: %w", err)
	}
	if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", actorID, "human", "reassigned", map[string]any{"old_assignee": oldAssigned.String, "new_assignee": req.AssignTo, "reason": req.Reason}); err != nil {
		return err
	}
	if err := s.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit reassign task tx: %w", err)
	}
	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, tenantID, caseID)
		email, _ := s.lookupPrincipalEmail(ctx, req.AssignTo)
		if err := s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "task_reassigned",
			TenantID:   tenantID,
			CaseID:     caseID,
			CaseNumber: caseNumber,
			StepID:     stepID,
			StepLabel:  stepID,
			Recipients: []notify.Recipient{{PrincipalID: req.AssignTo, Email: email, Channels: []string{"email", "websocket"}}},
			Data:       map[string]any{"reason": req.Reason},
		}); err != nil {
			slog.WarnContext(ctx, "task reassign notification failed", "tenant_id", tenantID.String(), "case_id", caseID.String(), "step_id", stepID, "error", err)
		}
	}
	return nil
}
