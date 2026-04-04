package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
)

func (s *TaskService) Inbox(ctx context.Context, tenantID, principalID uuid.UUID) ([]InboxTask, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
    cs.case_id,
    cs.step_id,
    c.case_number,
    ct.name,
    COALESCE(cs.metadata->>'label', cs.step_id) AS step_name,
    cs.assigned_to,
    c.priority,
    cs.started_at,
    cs.sla_deadline,
    COALESCE(cs.metadata, '{}'::jsonb)
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
JOIN case_types ct ON ct.id = c.case_type_id
WHERE cs.state = 'active'
  AND c.tenant_id = $1
  AND (
      cs.assigned_to = $2
      OR (
          cs.assigned_to IS NULL
          AND EXISTS (
              SELECT 1
              FROM principal_roles pr
              JOIN roles r ON r.id = pr.role_id
              WHERE pr.principal_id = $2
                AND r.tenant_id = $1
                AND r.name = COALESCE(cs.metadata->>'role', '')
          )
      )
  )
ORDER BY
  CASE WHEN cs.sla_deadline IS NOT NULL AND cs.sla_deadline < now() THEN 0 ELSE 1 END,
  cs.sla_deadline NULLS LAST,
  c.priority DESC,
  cs.started_at
`, tenantID, principalID)
	if err != nil {
		return nil, fmt.Errorf("query inbox tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]InboxTask, 0)
	now := s.now()
	for rows.Next() {
		var t InboxTask
		var assigned sql.NullString
		var started sql.NullTime
		var deadline sql.NullTime
		if err := rows.Scan(&t.CaseID, &t.StepID, &t.CaseNumber, &t.CaseType, &t.StepName, &assigned, &t.Priority, &started, &deadline, &t.Metadata); err != nil {
			return nil, fmt.Errorf("scan inbox task: %w", err)
		}
		if assigned.Valid {
			id, parseErr := uuid.Parse(assigned.String)
			if parseErr == nil {
				t.AssignedTo = &id
			}
		}
		if started.Valid {
			timeVal := started.Time
			t.StartedAt = &timeVal
		}
		if deadline.Valid {
			timeVal := deadline.Time
			t.SLADeadline = &timeVal
		}
		t.SLAHours = extractSLAHours(t.Metadata)
		t.SLAStatus = SLAStatus(now, t.SLADeadline, t.StartedAt, t.SLAHours)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool { return inboxLess(out[i], out[j], now) })
	return out, nil
}

func (s *TaskService) GetTask(ctx context.Context, tenantID uuid.UUID, caseID uuid.UUID, stepID string) (TaskDetail, error) {
	var d TaskDetail
	d.CaseID = caseID
	d.StepID = stepID
	var (
		caseDataRaw []byte
		resultRaw   []byte
		draftRaw    []byte
		metaRaw     []byte
		started     sql.NullTime
		deadline    sql.NullTime
		assigned    sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
SELECT c.case_number, ct.name, c.data, cs.state, cs.assigned_to, COALESCE(cs.result, '{}'::jsonb), COALESCE(cs.draft_data, 'null'::jsonb),
       COALESCE(cs.metadata, '{}'::jsonb), cs.started_at, cs.sla_deadline
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
JOIN case_types ct ON ct.id = c.case_type_id
WHERE cs.case_id = $1 AND cs.step_id = $2 AND c.tenant_id = $3
`, caseID, stepID, tenantID).Scan(&d.CaseNumber, &d.CaseType, &caseDataRaw, &d.StepState, &assigned, &resultRaw, &draftRaw, &metaRaw, &started, &deadline)
	if err != nil {
		return TaskDetail{}, err
	}
	if assigned.Valid {
		id, parseErr := uuid.Parse(assigned.String)
		if parseErr == nil {
			d.AssignedTo = &id
		}
	}
	_ = json.Unmarshal(caseDataRaw, &d.CaseData)
	d.Result = resultRaw
	d.DraftData = draftRaw
	_ = json.Unmarshal(metaRaw, &d.Metadata)
	if started.Valid {
		t := started.Time
		d.StartedAt = &t
	}
	if deadline.Valid {
		t := deadline.Time
		d.SLADeadline = &t
	}

	step, err := s.loadWorkflowStep(ctx, caseID, stepID)
	if err == nil {
		d.StepType = step.Type
		d.Outcomes = make([]string, 0, len(step.Outcomes))
		for k := range step.Outcomes {
			d.Outcomes = append(d.Outcomes, k)
		}
		sort.Strings(d.Outcomes)
		d.AvailableActions = append([]string(nil), d.Outcomes...)
		cfg := AssignmentConfig{}
		_ = json.Unmarshal(step.Config, &cfg)
		d.Form = cfg.Form
		d.FormSchema = cfg.FormSchema
	}
	if outcomes := configuredOutcomes(d.Metadata); len(outcomes) > 0 {
		d.Outcomes = outcomes
		d.AvailableActions = append([]string(nil), outcomes...)
	}
	if schema := configuredFormSchema(d.Metadata); len(schema.Fields) > 0 {
		d.FormSchema = schema
		if d.Form == "" {
			if v, ok := d.Metadata["form"].(string); ok {
				d.Form = v
			}
		}
	}

	d.StepResults = map[string]any{}
	rows, qerr := s.db.QueryContext(ctx, `
SELECT cs.step_id, COALESCE(cs.result, '{}'::jsonb)
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE cs.case_id = $1
  AND cs.state = 'completed'
  AND c.tenant_id = $2
`, caseID, tenantID)
	if qerr == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var sid string
			var raw []byte
			if scanErr := rows.Scan(&sid, &raw); scanErr == nil {
				var payload any
				if unErr := json.Unmarshal(raw, &payload); unErr == nil {
					d.StepResults[sid] = payload
				}
			}
		}
	}
	d.SLAStatus = SLAStatus(s.now(), d.SLADeadline, d.StartedAt, extractSLAHours(metaRaw))
	return d, nil
}
