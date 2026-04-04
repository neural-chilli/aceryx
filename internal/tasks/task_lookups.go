package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/notify"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (s *TaskService) lookupTenantID(ctx context.Context, caseID uuid.UUID) (uuid.UUID, error) {
	var tenantID uuid.UUID
	if expectedTenant := observability.TenantIDFromContext(ctx); expectedTenant != "" {
		if parsedExpected, err := uuid.Parse(expectedTenant); err == nil {
			err = s.db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1 AND tenant_id = $2`, caseID, parsedExpected).Scan(&tenantID)
			return tenantID, err
		}
	}
	err := s.db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1`, caseID).Scan(&tenantID)
	return tenantID, err
}

func (s *TaskService) lookupCaseNumber(ctx context.Context, tenantID, caseID uuid.UUID) (string, error) {
	var caseNumber string
	err := s.db.QueryRowContext(ctx, `SELECT case_number FROM cases WHERE id = $1 AND tenant_id = $2`, caseID, tenantID).Scan(&caseNumber)
	return caseNumber, err
}

func (s *TaskService) lookupPrincipalEmail(ctx context.Context, principalID uuid.UUID) (string, error) {
	var email sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT email FROM principals WHERE id = $1 AND status = 'active'`, principalID).Scan(&email); err != nil {
		return "", err
	}
	return email.String, nil
}

func (s *TaskService) lookupCaseAssignee(ctx context.Context, tenantID, caseID uuid.UUID) (*uuid.UUID, error) {
	var assigned sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT assigned_to FROM cases WHERE tenant_id = $1 AND id = $2`, tenantID, caseID).Scan(&assigned); err != nil {
		return nil, err
	}
	if !assigned.Valid {
		return nil, nil
	}
	id, err := uuid.Parse(assigned.String)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *TaskService) lookupStepRole(ctx context.Context, caseID uuid.UUID, stepID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(metadata->>'role', '')
FROM case_steps
WHERE case_id = $1 AND step_id = $2
`, caseID, stepID).Scan(&role)
	if err != nil {
		return "", err
	}
	return role, nil
}

func (s *TaskService) resolveRoleRecipients(ctx context.Context, tenantID uuid.UUID, role string, channels []string) ([]notify.Recipient, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, COALESCE(p.email, '')
FROM principals p
JOIN principal_roles pr ON pr.principal_id = p.id
JOIN roles r ON r.id = pr.role_id
WHERE p.tenant_id = $1
  AND p.status = 'active'
  AND r.name = $2
`, tenantID, role)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]notify.Recipient, 0)
	for rows.Next() {
		var (
			id    uuid.UUID
			email string
		)
		if err := rows.Scan(&id, &email); err != nil {
			return nil, err
		}
		out = append(out, notify.Recipient{PrincipalID: id, Email: email, Channels: channels})
	}
	return out, rows.Err()
}

func dedupeRecipients(in []notify.Recipient) []notify.Recipient {
	type key struct {
		id uuid.UUID
	}
	merged := map[key]notify.Recipient{}
	for _, rec := range in {
		k := key{id: rec.PrincipalID}
		cur, ok := merged[k]
		if !ok {
			merged[k] = rec
			continue
		}
		channelSet := map[string]struct{}{}
		for _, ch := range cur.Channels {
			channelSet[ch] = struct{}{}
		}
		for _, ch := range rec.Channels {
			if _, exists := channelSet[ch]; !exists {
				cur.Channels = append(cur.Channels, ch)
				channelSet[ch] = struct{}{}
			}
		}
		if cur.Email == "" {
			cur.Email = rec.Email
		}
		merged[k] = cur
	}
	out := make([]notify.Recipient, 0, len(merged))
	for _, rec := range merged {
		out = append(out, rec)
	}
	return out
}

func (s *TaskService) loadWorkflowStep(ctx context.Context, caseID uuid.UUID, stepID string) (engine.WorkflowStep, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT wv.ast
FROM cases c
JOIN workflow_versions wv ON wv.workflow_id = c.workflow_id AND wv.version = c.workflow_version
WHERE c.id = $1
`, caseID).Scan(&raw)
	if err != nil {
		return engine.WorkflowStep{}, err
	}
	var ast engine.WorkflowAST
	if err := json.Unmarshal(raw, &ast); err != nil {
		return engine.WorkflowStep{}, err
	}
	for _, step := range ast.Steps {
		if step.ID == stepID {
			return step, nil
		}
	}
	return engine.WorkflowStep{}, sql.ErrNoRows
}

func (s *TaskService) systemActor() uuid.UUID {
	if s.sysActor != uuid.Nil {
		return s.sysActor
	}
	return uuid.MustParse("00000000-0000-0000-0000-000000000000")
}

func (s *TaskService) scheduleSLAWarning(caseID uuid.UUID, stepID string, tenantID uuid.UUID, assignedTo uuid.UUID, slaHours int) {
	if s.notify == nil || s.after == nil || slaHours <= 0 {
		return
	}
	total := time.Duration(slaHours) * time.Hour
	delay := total - (total / 4)
	if delay <= 0 {
		return
	}
	s.after(delay, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var (
			state      string
			caseNumber string
			current    sql.NullString
		)
		err := s.db.QueryRowContext(ctx, `
SELECT cs.state, c.case_number, cs.assigned_to
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE c.tenant_id = $1 AND cs.case_id = $2 AND cs.step_id = $3
`, tenantID, caseID, stepID).Scan(&state, &caseNumber, &current)
		if err != nil || state != engine.StateActive || !current.Valid {
			return
		}
		currentID, err := uuid.Parse(current.String)
		if err != nil || currentID != assignedTo {
			return
		}
		_ = s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "sla_warning",
			TenantID:   tenantID,
			CaseID:     caseID,
			CaseNumber: caseNumber,
			StepID:     stepID,
			StepLabel:  stepID,
			Recipients: []notify.Recipient{{PrincipalID: assignedTo, Channels: []string{"websocket"}}},
			Data:       map[string]any{},
		})
	})
}
