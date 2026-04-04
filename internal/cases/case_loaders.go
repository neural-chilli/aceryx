package cases

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/notify"
)

func (s *CaseService) loadCaseSteps(ctx context.Context, caseID uuid.UUID) ([]CaseStep, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, step_id, state, started_at, completed_at,
       COALESCE(result, '{}'::jsonb),
       COALESCE(events, '[]'::jsonb),
       COALESCE(error, '{}'::jsonb),
       assigned_to,
       sla_deadline,
       retry_count,
       COALESCE(draft_data, '{}'::jsonb),
       COALESCE(metadata, '{}'::jsonb)
FROM case_steps
WHERE case_id = $1
ORDER BY step_id
`, caseID)
	if err != nil {
		return nil, fmt.Errorf("load case steps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseStep, 0)
	for rows.Next() {
		var step CaseStep
		if err := rows.Scan(&step.ID, &step.StepID, &step.State, &step.StartedAt, &step.CompletedAt,
			&step.Result, &step.Events, &step.Error, &step.AssignedTo, &step.SLADeadline,
			&step.RetryCount, &step.DraftData, &step.Metadata); err != nil {
			return nil, fmt.Errorf("scan case step: %w", err)
		}
		out = append(out, step)
	}
	return out, rows.Err()
}

func (s *CaseService) loadCaseEvents(ctx context.Context, caseID uuid.UUID) ([]CaseEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, COALESCE(step_id, ''), event_type, actor_id, actor_type, action, data, created_at, prev_event_hash, event_hash
FROM case_events
WHERE case_id = $1
ORDER BY created_at
`, caseID)
	if err != nil {
		return nil, fmt.Errorf("load case events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseEvent, 0)
	for rows.Next() {
		var (
			event CaseEvent
			raw   []byte
		)
		if err := rows.Scan(&event.ID, &event.StepID, &event.EventType, &event.ActorID, &event.ActorType, &event.Action, &raw, &event.CreatedAt, &event.PrevEventHash, &event.EventHash); err != nil {
			return nil, fmt.Errorf("scan case event: %w", err)
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &event.Data)
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *CaseService) loadCaseDocuments(ctx context.Context, caseID uuid.UUID) ([]CaseDocument, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, filename, mime_type, size_bytes, uploaded_by, uploaded_at, deleted_at
FROM vault_documents
WHERE case_id = $1
ORDER BY uploaded_at DESC
`, caseID)
	if err != nil {
		return nil, fmt.Errorf("load case documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseDocument, 0)
	for rows.Next() {
		var d CaseDocument
		if err := rows.Scan(&d.ID, &d.Filename, &d.MimeType, &d.SizeBytes, &d.UploadedBy, &d.UploadedAt, &d.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan case document: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *CaseService) caseCreator(ctx context.Context, tenantID, caseID uuid.UUID) (caseNumber string, creatorID uuid.UUID, creatorEmail string, err error) {
	err = s.db.QueryRowContext(ctx, `
SELECT c.case_number, c.created_by, COALESCE(p.email, '')
FROM cases c
JOIN principals p ON p.id = c.created_by
WHERE c.tenant_id = $1 AND c.id = $2
`, tenantID, caseID).Scan(&caseNumber, &creatorID, &creatorEmail)
	return caseNumber, creatorID, creatorEmail, err
}

func (s *CaseService) caseCancellationRecipients(ctx context.Context, tenantID, caseID uuid.UUID) (string, []notify.Recipient, error) {
	caseNumber, creatorID, creatorEmail, err := s.caseCreator(ctx, tenantID, caseID)
	if err != nil {
		return "", nil, err
	}
	recipients := []notify.Recipient{{PrincipalID: creatorID, Email: creatorEmail, Channels: []string{"email", "websocket"}}}

	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT p.id, COALESCE(p.email, '')
FROM case_steps cs
JOIN principals p ON p.id = cs.assigned_to
JOIN cases c ON c.id = cs.case_id
WHERE c.tenant_id = $1
  AND cs.case_id = $2
  AND cs.assigned_to IS NOT NULL
  AND p.status = 'active'
`, tenantID, caseID)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = rows.Close() }()

	seen := map[uuid.UUID]struct{}{creatorID: {}}
	for rows.Next() {
		var (
			id    uuid.UUID
			email string
		)
		if err := rows.Scan(&id, &email); err != nil {
			return "", nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		recipients = append(recipients, notify.Recipient{PrincipalID: id, Email: email, Channels: []string{"email", "websocket"}})
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	return caseNumber, recipients, nil
}
