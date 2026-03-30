package tasks

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
	"github.com/neural-chilli/aceryx/internal/engine"
)

var (
	ErrAlreadyClaimed   = errors.New("task already claimed")
	ErrAlreadyCompleted = errors.New("task already completed")
	ErrForbidden        = errors.New("forbidden")
	ErrInvalidOutcome   = errors.New("invalid outcome")
)

type Notifier interface {
	NotifyUser(ctx context.Context, principalID uuid.UUID, payload map[string]any) error
	NotifyRole(ctx context.Context, tenantID uuid.UUID, role string, payload map[string]any) error
}

type Engine interface {
	EvaluateDAG(ctx context.Context, caseID uuid.UUID) error
}

type TaskService struct {
	db       *sql.DB
	notify   Notifier
	engine   Engine
	now      func() time.Time
	sysActor uuid.UUID
}

type userLoad struct {
	PrincipalID uuid.UUID
	ActiveCount int
	CreatedAt   time.Time
}

type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type EscalationConfig struct {
	AfterHours int    `json:"after_hours"`
	ToRole     string `json:"to_role"`
	Action     string `json:"action"`
}

type AssignmentConfig struct {
	AssignToRole string           `json:"assign_to_role"`
	AssignToUser string           `json:"assign_to_user"`
	SLAHours     int              `json:"sla_hours"`
	Escalation   EscalationConfig `json:"escalation"`
	Form         string           `json:"form"`
	FormSchema   FormSchema       `json:"form_schema"`
}

type FormSchema struct {
	Fields []FormField `json:"fields"`
}

type FormField struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Bind     string `json:"bind"`
}

type InboxTask struct {
	CaseID      uuid.UUID  `json:"case_id"`
	StepID      string     `json:"step_id"`
	CaseNumber  string     `json:"case_number"`
	CaseType    string     `json:"case_type"`
	StepName    string     `json:"step_name"`
	AssignedTo  *uuid.UUID `json:"assigned_to,omitempty"`
	Priority    int        `json:"priority"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	SLADeadline *time.Time `json:"sla_deadline,omitempty"`
	SLAStatus   string     `json:"sla_status"`
	Metadata    json.RawMessage
	SLAHours    int `json:"sla_hours"`
}

type TaskDetail struct {
	CaseID           uuid.UUID         `json:"case_id"`
	StepID           string            `json:"step_id"`
	CaseNumber       string            `json:"case_number"`
	CaseType         string            `json:"case_type"`
	CaseData         map[string]any    `json:"case_data"`
	StepState        string            `json:"state"`
	AssignedTo       *uuid.UUID        `json:"assigned_to,omitempty"`
	Result           json.RawMessage   `json:"result,omitempty"`
	DraftData        json.RawMessage   `json:"draft_data,omitempty"`
	Form             string            `json:"form"`
	FormSchema       FormSchema        `json:"form_schema"`
	Outcomes         []string          `json:"outcomes"`
	AvailableActions []string          `json:"available_actions"`
	StepResults      map[string]any    `json:"step_results"`
	Metadata         map[string]any    `json:"metadata"`
	StartedAt        *time.Time        `json:"started_at,omitempty"`
	SLADeadline      *time.Time        `json:"sla_deadline,omitempty"`
	SLAStatus        string            `json:"sla_status"`
	Validation       []ValidationError `json:"validation,omitempty"`
}

type CompleteTaskRequest struct {
	Outcome string         `json:"outcome"`
	Data    map[string]any `json:"data"`
}

type ReassignRequest struct {
	AssignTo uuid.UUID `json:"assign_to"`
	Reason   string    `json:"reason"`
}

type DraftRequest struct {
	Data map[string]any `json:"data"`
}

func NewTaskService(db *sql.DB, eng Engine, notify Notifier) *TaskService {
	return &TaskService{db: db, engine: eng, notify: notify, now: func() time.Time { return time.Now().UTC() }, sysActor: uuid.Nil}
}

func (s *TaskService) SetSystemActorID(id uuid.UUID) {
	s.sysActor = id
}

func (s *TaskService) CreateTaskFromActivation(ctx context.Context, caseID uuid.UUID, stepID string, cfg AssignmentConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin task creation tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := s.now()
	var assignedTo *uuid.UUID
	if cfg.AssignToUser != "" {
		uid, parseErr := uuid.Parse(cfg.AssignToUser)
		if parseErr == nil {
			assignedTo = &uid
		}
	}

	var deadline any
	if cfg.SLAHours > 0 {
		d := now.Add(time.Duration(cfg.SLAHours) * time.Hour)
		deadline = d
	}

	metadata := map[string]any{"role": cfg.AssignToRole, "form": cfg.Form, "sla_hours": cfg.SLAHours, "escalation": cfg.Escalation}
	rawMeta, _ := json.Marshal(metadata)
	if _, err := tx.ExecContext(ctx, `
UPDATE case_steps
SET
    state='active',
    started_at = COALESCE(started_at, now()),
    assigned_to = $3,
    sla_deadline = $4,
    metadata = COALESCE(metadata, '{}'::jsonb) || $5::jsonb
WHERE case_id = $1 AND step_id = $2
`, caseID, stepID, assignedTo, deadline, string(rawMeta)); err != nil {
		return fmt.Errorf("activate human task step: %w", err)
	}

	actor := s.systemActor()
	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task_created", actor, "system", "task_create", map[string]any{"assigned_to": assignedTo, "role": cfg.AssignToRole, "sla_hours": cfg.SLAHours}); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case timestamp task create: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task creation: %w", err)
	}

	payload := map[string]any{"type": "task_update", "action": "created", "case_id": caseID, "step_id": stepID}
	if s.notify != nil {
		if assignedTo != nil {
			_ = s.notify.NotifyUser(ctx, *assignedTo, payload)
		} else if cfg.AssignToRole != "" {
			tenantID, _ := s.lookupTenantID(ctx, caseID)
			_ = s.notify.NotifyRole(ctx, tenantID, cfg.AssignToRole, payload)
		}
	}
	return nil
}

func (s *TaskService) Inbox(ctx context.Context, tenantID, principalID uuid.UUID) ([]InboxTask, error) {
	roles, err := s.principalRoleNames(ctx, tenantID, principalID)
	if err != nil {
		return nil, err
	}

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
          AND COALESCE(cs.metadata->>'role', '') = ANY($3)
      )
  )
ORDER BY
  CASE WHEN cs.sla_deadline IS NOT NULL AND cs.sla_deadline < now() THEN 0 ELSE 1 END,
  cs.sla_deadline NULLS LAST,
  c.priority DESC,
  cs.started_at
`, tenantID, principalID, pqStringArray(roles))
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

	d.StepResults = map[string]any{}
	rows, qerr := s.db.QueryContext(ctx, `SELECT step_id, COALESCE(result, '{}'::jsonb) FROM case_steps WHERE case_id = $1 AND state = 'completed'`, caseID)
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

func (s *TaskService) ClaimTask(ctx context.Context, tenantID, principalID, caseID uuid.UUID, stepID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin claim task tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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
    RETURNING 1
)
SELECT EXISTS(SELECT 1 FROM claim)
`, caseID, stepID, tenantID, principalID).Scan(&claimed)
	if err != nil {
		return fmt.Errorf("claim task: %w", err)
	}
	if !claimed {
		return ErrAlreadyClaimed
	}

	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task_claimed", principalID, "human", "task_claim", map[string]any{}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case task claim: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit claim task: %w", err)
	}

	if s.notify != nil {
		_ = s.notify.NotifyRole(ctx, tenantID, "*", map[string]any{"type": "task_update", "action": "claimed", "case_id": caseID, "step_id": stepID})
	}
	return nil
}

func (s *TaskService) SaveDraft(ctx context.Context, tenantID, principalID, caseID uuid.UUID, stepID string, req DraftRequest) error {
	dataRaw, _ := json.Marshal(req.Data)
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
	taskDetail, err := s.GetTask(ctx, tenantID, caseID, stepID)
	if err != nil {
		return err
	}
	if taskDetail.StepState == engine.StateCompleted {
		return ErrAlreadyCompleted
	}
	if taskDetail.AssignedTo == nil || *taskDetail.AssignedTo != principalID {
		return ErrForbidden
	}
	if !contains(taskDetail.Outcomes, req.Outcome) {
		return ErrInvalidOutcome
	}
	validation := ValidateFormData(taskDetail.FormSchema, req.Data)
	if len(validation) > 0 {
		return fmt.Errorf("validation_failed: %s", validation[0].Message)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin complete task tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	resultRaw, _ := json.Marshal(map[string]any{"outcome": req.Outcome, "data": req.Data})
	res, err := tx.ExecContext(ctx, `
UPDATE case_steps cs
SET state='completed', completed_at=now(), result=$6::jsonb, draft_data=NULL,
    events = COALESCE(events, '[]'::jsonb) || jsonb_build_array(jsonb_build_object('type','completed','outcome',$7,'at',now()))
FROM cases c
WHERE cs.case_id=$1 AND cs.step_id=$2 AND c.id=cs.case_id AND c.tenant_id=$3 AND cs.state='active' AND cs.assigned_to=$4
`, caseID, stepID, tenantID, principalID, s.now(), string(resultRaw), req.Outcome)
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

	decisionPatch := buildDecisionPatch(taskDetail.FormSchema, req.Data)
	if len(decisionPatch) > 0 {
		patchRaw, _ := json.Marshal(decisionPatch)
		if _, err := tx.ExecContext(ctx, `
UPDATE cases
SET data = COALESCE(data, '{}'::jsonb) || jsonb_build_object('decision', COALESCE(data->'decision', '{}'::jsonb) || $2::jsonb),
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

	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task_completed", principalID, "human", "task_complete", map[string]any{"outcome": req.Outcome, "data": req.Data}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit complete task tx: %w", err)
	}

	if s.engine != nil {
		_ = s.engine.EvaluateDAG(ctx, caseID)
	}
	if s.notify != nil {
		_ = s.notify.NotifyRole(ctx, tenantID, "*", map[string]any{"type": "task_update", "action": "completed", "case_id": caseID, "step_id": stepID})
	}
	return nil
}

func (s *TaskService) ReassignTask(ctx context.Context, tenantID, actorID, caseID uuid.UUID, stepID string, req ReassignRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reassign task tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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

	if _, err := tx.ExecContext(ctx, `UPDATE case_steps SET assigned_to=$4 WHERE case_id=$1 AND step_id=$2`, caseID, stepID, tenantID, req.AssignTo); err != nil {
		return fmt.Errorf("reassign task: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id=$1`, caseID); err != nil {
		return fmt.Errorf("touch case reassign: %w", err)
	}
	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task_reassigned", actorID, "human", "task_reassign", map[string]any{"old_assignee": oldAssigned.String, "new_assignee": req.AssignTo, "reason": req.Reason}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reassign task tx: %w", err)
	}
	if s.notify != nil {
		_ = s.notify.NotifyUser(ctx, req.AssignTo, map[string]any{"type": "task_update", "action": "reassigned", "case_id": caseID, "step_id": stepID})
	}
	return nil
}

func (s *TaskService) EscalateTask(ctx context.Context, tenantID uuid.UUID, caseID uuid.UUID, stepID string, cfg EscalationConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin escalate task tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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
		if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task_escalation_suppressed", s.systemActor(), "system", "task_escalation_suppressed", map[string]any{"state": state}); err != nil {
			return err
		}
		return tx.Commit()
	}

	var reassignedTo *uuid.UUID
	if cfg.Action == "reassign" || cfg.Action == "both" {
		uid, err := s.pickLeastLoadedUserTx(ctx, tx, tenantID, cfg.ToRole)
		if err == nil && uid != uuid.Nil {
			reassignedTo = &uid
			if _, err := tx.ExecContext(ctx, `UPDATE case_steps SET assigned_to=$4 WHERE case_id=$1 AND step_id=$2`, caseID, stepID, tenantID, uid); err != nil {
				return fmt.Errorf("escalation reassign: %w", err)
			}
		}
	}

	eventType := "task_escalated"
	if cfg.Action == "notify" {
		eventType = "task_escalation_notified"
	}
	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, eventType, s.systemActor(), "system", "task_escalate", map[string]any{"to_role": cfg.ToRole, "action": cfg.Action, "assigned_to": reassignedTo}); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id=$1`, caseID); err != nil {
		return fmt.Errorf("touch case escalate: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit escalate tx: %w", err)
	}

	if s.notify != nil && cfg.ToRole != "" {
		_ = s.notify.NotifyRole(ctx, tenantID, cfg.ToRole, map[string]any{"type": "task_update", "action": "escalated", "case_id": caseID, "step_id": stepID})
	}
	if s.notify != nil && reassignedTo != nil {
		_ = s.notify.NotifyUser(ctx, *reassignedTo, map[string]any{"type": "task_update", "action": "escalated_reassign", "case_id": caseID, "step_id": stepID})
	}
	return nil
}

func (s *TaskService) HandleOverdue(ctx context.Context, task engine.OverdueTask) error {
	cfg, tenantID, err := s.loadEscalationConfig(ctx, task.CaseID, task.StepID)
	if err != nil {
		return err
	}
	if cfg.Action == "" {
		return nil
	}
	return s.EscalateTask(ctx, tenantID, task.CaseID, task.StepID, cfg)
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
	defer rows.Close()
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

func (s *TaskService) principalRoleNames(ctx context.Context, tenantID, principalID uuid.UUID) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.name
FROM principal_roles pr
JOIN roles r ON r.id = pr.role_id
JOIN principals p ON p.id = pr.principal_id
WHERE pr.principal_id=$1 AND p.tenant_id=$2
`, principalID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query principal roles: %w", err)
	}
	defer rows.Close()
	roles := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

func (s *TaskService) lookupTenantID(ctx context.Context, caseID uuid.UUID) (uuid.UUID, error) {
	var tenantID uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id=$1`, caseID).Scan(&tenantID)
	return tenantID, err
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

func extractSLAHours(metaRaw []byte) int {
	var meta map[string]any
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return 0
	}
	if v, ok := meta["sla_hours"].(float64); ok {
		return int(v)
	}
	return 0
}

func SLAStatus(now time.Time, deadline *time.Time, startedAt *time.Time, slaHours int) string {
	if deadline == nil {
		return "on_track"
	}
	if deadline.Before(now) {
		return "breached"
	}
	if slaHours > 0 {
		warningWindow := time.Duration(float64(time.Hour) * float64(slaHours) * 0.25)
		warningStart := deadline.Add(-warningWindow)
		if !now.Before(warningStart) {
			return "warning"
		}
		return "on_track"
	}
	if startedAt != nil {
		remaining := deadline.Sub(now)
		full := deadline.Sub(*startedAt)
		if full > 0 && remaining <= full/4 {
			return "warning"
		}
	}
	return "on_track"
}

func ValidateFormData(schema FormSchema, data map[string]any) []ValidationError {
	errs := make([]ValidationError, 0)
	for _, field := range schema.Fields {
		v, ok := data[field.ID]
		if field.Required && !ok {
			errs = append(errs, ValidationError{Field: field.ID, Code: "required", Message: "field is required"})
			continue
		}
		if !ok {
			continue
		}
		switch strings.ToLower(field.Type) {
		case "string":
			if _, ok := v.(string); !ok {
				errs = append(errs, ValidationError{Field: field.ID, Code: "type", Message: "must be a string"})
			}
		case "number":
			switch v.(type) {
			case float64, float32, int, int64, int32, uint, uint64, json.Number:
			default:
				errs = append(errs, ValidationError{Field: field.ID, Code: "type", Message: "must be a number"})
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				errs = append(errs, ValidationError{Field: field.ID, Code: "type", Message: "must be a boolean"})
			}
		case "object":
			if _, ok := v.(map[string]any); !ok {
				errs = append(errs, ValidationError{Field: field.ID, Code: "type", Message: "must be an object"})
			}
		}
	}
	return errs
}

func buildDecisionPatch(schema FormSchema, data map[string]any) map[string]any {
	out := map[string]any{}
	for _, field := range schema.Fields {
		if !strings.HasPrefix(field.Bind, "decision.") {
			continue
		}
		val, ok := data[field.ID]
		if !ok {
			continue
		}
		key := strings.TrimPrefix(field.Bind, "decision.")
		out[key] = val
	}
	return out
}

func contains(items []string, val string) bool {
	for _, item := range items {
		if item == val {
			return true
		}
	}
	return false
}

func inboxLess(a, b InboxTask, now time.Time) bool {
	as := SLAStatus(now, a.SLADeadline, a.StartedAt, a.SLAHours)
	bs := SLAStatus(now, b.SLADeadline, b.StartedAt, b.SLAHours)
	order := map[string]int{"breached": 0, "warning": 1, "on_track": 2}
	if order[as] != order[bs] {
		return order[as] < order[bs]
	}
	if a.SLADeadline != nil && b.SLADeadline != nil && !a.SLADeadline.Equal(*b.SLADeadline) {
		return a.SLADeadline.Before(*b.SLADeadline)
	}
	if a.SLADeadline == nil && b.SLADeadline != nil {
		return false
	}
	if a.SLADeadline != nil && b.SLADeadline == nil {
		return true
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if a.StartedAt != nil && b.StartedAt != nil {
		return a.StartedAt.Before(*b.StartedAt)
	}
	if a.StartedAt == nil && b.StartedAt != nil {
		return false
	}
	if a.StartedAt != nil && b.StartedAt == nil {
		return true
	}
	return a.StepID < b.StepID
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

// HumanTaskExecutor activates a long-running human task and returns immediately.
type HumanTaskExecutor struct {
	svc *TaskService
}

func NewHumanTaskExecutor(svc *TaskService) *HumanTaskExecutor {
	return &HumanTaskExecutor{svc: svc}
}

func (e *HumanTaskExecutor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, config json.RawMessage) (*engine.StepResult, error) {
	cfg := AssignmentConfig{}
	if len(config) > 0 {
		_ = json.Unmarshal(config, &cfg)
	}
	if err := e.svc.CreateTaskFromActivation(ctx, caseID, stepID, cfg); err != nil {
		return nil, err
	}
	return &engine.StepResult{Output: json.RawMessage(`{"task_created":true}`)}, nil
}

// minimal pg text[] array wrapper without new dependency
type pqStringArray []string

func (a pqStringArray) Value() (driver.Value, error) {
	quoted := make([]string, len(a))
	for i, v := range a {
		quoted[i] = `"` + strings.ReplaceAll(v, `"`, `\\"`) + `"`
	}
	return "{" + strings.Join(quoted, ",") + "}", nil
}
