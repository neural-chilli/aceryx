package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/notify"
	"github.com/neural-chilli/aceryx/internal/observability"
)

var (
	ErrAlreadyClaimed   = errors.New("task already claimed")
	ErrAlreadyCompleted = errors.New("task already completed")
	ErrForbidden        = errors.New("forbidden")
	ErrInvalidOutcome   = errors.New("invalid outcome")
)

type Notifier interface {
	Notify(ctx context.Context, event notify.NotifyEvent) error
}

type Engine interface {
	EvaluateDAG(ctx context.Context, caseID uuid.UUID) error
}

type TaskService struct {
	db       *sql.DB
	notify   Notifier
	engine   Engine
	now      func() time.Time
	after    func(time.Duration, func()) *time.Timer
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
	Outcomes     []string         `json:"outcomes"`
	Metadata     map[string]any   `json:"metadata"`
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
	StepType         string            `json:"step_type,omitempty"`
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
	return &TaskService{
		db:     db,
		engine: eng,
		notify: notify,
		now: func() time.Time {
			return time.Now().UTC()
		},
		after:    time.AfterFunc,
		sysActor: uuid.Nil,
	}
}

func (s *TaskService) SetSystemActorID(id uuid.UUID) {
	s.sysActor = id
}

func (s *TaskService) CreateTaskFromActivation(ctx context.Context, caseID uuid.UUID, stepID string, cfg AssignmentConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin task creation tx: %w", err)
	}
	defer func() { _ = audit.RollbackTx(tx) }()

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
	if len(cfg.FormSchema.Fields) > 0 {
		metadata["form_schema"] = cfg.FormSchema
	}
	if len(cfg.Outcomes) > 0 {
		metadata["outcomes"] = cfg.Outcomes
	}
	for k, v := range cfg.Metadata {
		metadata[k] = v
	}
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
	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", actor, "system", "created", map[string]any{"assigned_to": assignedTo, "role": cfg.AssignToRole, "sla_hours": cfg.SLAHours}); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case timestamp task create: %w", err)
	}

	if err := audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit task creation: %w", err)
	}
	tenantID, _ := s.lookupTenantID(ctx, caseID)
	if tenantID != uuid.Nil {
		s.updateActiveTasksGauge(ctx, tenantID)
	}
	slog.InfoContext(ctx, "task created",
		append(observability.RequestAttrs(ctx),
			"tenant_id", tenantID.String(),
			"case_id", caseID.String(),
			"step_id", stepID,
		)...,
	)

	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, caseID)
		stepLabel := stepID
		if v, ok := metadata["label"].(string); ok && strings.TrimSpace(v) != "" {
			stepLabel = v
		}
		if assignedTo != nil {
			email, _ := s.lookupPrincipalEmail(ctx, *assignedTo)
			_ = s.notify.Notify(ctx, notify.NotifyEvent{
				Type:       "task_assigned",
				TenantID:   tenantID,
				CaseID:     caseID,
				CaseNumber: caseNumber,
				StepID:     stepID,
				StepLabel:  stepLabel,
				Recipients: []notify.Recipient{{PrincipalID: *assignedTo, Email: email, Channels: []string{"email", "websocket"}}},
				Data:       map[string]any{},
			})
		} else if cfg.AssignToRole != "" {
			recipients, _ := s.resolveRoleRecipients(ctx, tenantID, cfg.AssignToRole, []string{"websocket"})
			_ = s.notify.Notify(ctx, notify.NotifyEvent{
				Type:       "task_assigned",
				TenantID:   tenantID,
				CaseID:     caseID,
				CaseNumber: caseNumber,
				StepID:     stepID,
				StepLabel:  stepLabel,
				Recipients: recipients,
				Data:       map[string]any{},
			})
		}
	}
	if assignedTo != nil && cfg.SLAHours > 0 {
		s.scheduleSLAWarning(caseID, stepID, tenantID, *assignedTo, cfg.SLAHours)
	}
	return nil
}

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
	defer func() { _ = audit.RollbackTx(tx) }()

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

	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", principalID, "human", "claimed", map[string]any{}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case task claim: %w", err)
	}
	if err := audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit claim task: %w", err)
	}

	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, caseID)
		roleName, _ := s.lookupStepRole(ctx, caseID, stepID)
		recipients := make([]notify.Recipient, 0)
		if roleName != "" {
			recipients, _ = s.resolveRoleRecipients(ctx, tenantID, roleName, []string{"websocket"})
		}
		_ = s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "task_claimed",
			TenantID:   tenantID,
			CaseID:     caseID,
			CaseNumber: caseNumber,
			StepID:     stepID,
			StepLabel:  stepID,
			Recipients: recipients,
			Data:       map[string]any{"claimed_by": principalID.String()},
		})
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
	if len(validation) > 0 {
		return fmt.Errorf("validation_failed: %s", validation[0].Message)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin complete task tx: %w", err)
	}
	defer func() { _ = audit.RollbackTx(tx) }()

	resultRaw, _ := json.Marshal(map[string]any{"outcome": req.Outcome, "data": payload})
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

	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", principalID, "human", "completed", map[string]any{"outcome": req.Outcome, "data": payload}); err != nil {
		return err
	}

	if err := audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit complete task tx: %w", err)
	}

	if s.engine != nil {
		_ = s.engine.EvaluateDAG(ctx, caseID)
	}
	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, caseID)
		caseAssignee, _ := s.lookupCaseAssignee(ctx, tenantID, caseID)
		recipients := make([]notify.Recipient, 0)
		if caseAssignee != nil && *caseAssignee != principalID {
			email, _ := s.lookupPrincipalEmail(ctx, *caseAssignee)
			recipients = append(recipients, notify.Recipient{PrincipalID: *caseAssignee, Email: email, Channels: []string{"websocket"}})
		}
		_ = s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "task_completed",
			TenantID:   tenantID,
			CaseID:     caseID,
			CaseNumber: caseNumber,
			StepID:     stepID,
			StepLabel:  stepID,
			Recipients: recipients,
			Data:       map[string]any{"outcome": req.Outcome, "completed_by": principalID.String()},
		})
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
	defer func() { _ = audit.RollbackTx(tx) }()

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
	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", actorID, "human", "reassigned", map[string]any{"old_assignee": oldAssigned.String, "new_assignee": req.AssignTo, "reason": req.Reason}); err != nil {
		return err
	}
	if err := audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit reassign task tx: %w", err)
	}
	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, caseID)
		email, _ := s.lookupPrincipalEmail(ctx, req.AssignTo)
		_ = s.notify.Notify(ctx, notify.NotifyEvent{
			Type:       "task_reassigned",
			TenantID:   tenantID,
			CaseID:     caseID,
			CaseNumber: caseNumber,
			StepID:     stepID,
			StepLabel:  stepID,
			Recipients: []notify.Recipient{{PrincipalID: req.AssignTo, Email: email, Channels: []string{"email", "websocket"}}},
			Data:       map[string]any{"reason": req.Reason},
		})
	}
	return nil
}

func (s *TaskService) EscalateTask(ctx context.Context, tenantID uuid.UUID, caseID uuid.UUID, stepID string, cfg EscalationConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin escalate task tx: %w", err)
	}
	defer func() { _ = audit.RollbackTx(tx) }()

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
		if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", s.systemActor(), "system", "escalation_suppressed", map[string]any{"state": state}); err != nil {
			return err
		}
		return audit.CommitTx(tx)
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

	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", s.systemActor(), "system", "escalated", map[string]any{"to_role": cfg.ToRole, "action": cfg.Action, "assigned_to": reassignedTo}); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id=$1`, caseID); err != nil {
		return fmt.Errorf("touch case escalate: %w", err)
	}
	if err := audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit escalate tx: %w", err)
	}

	if s.notify != nil {
		caseNumber, _ := s.lookupCaseNumber(ctx, caseID)
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
		caseNumber, _ := s.lookupCaseNumber(ctx, task.CaseID)
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

func (s *TaskService) lookupTenantID(ctx context.Context, caseID uuid.UUID) (uuid.UUID, error) {
	var tenantID uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id=$1`, caseID).Scan(&tenantID)
	return tenantID, err
}

func (s *TaskService) lookupCaseNumber(ctx context.Context, caseID uuid.UUID) (string, error) {
	var caseNumber string
	err := s.db.QueryRowContext(ctx, `SELECT case_number FROM cases WHERE id = $1`, caseID).Scan(&caseNumber)
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

func configuredOutcomes(metadata map[string]any) []string {
	raw, ok := metadata["outcomes"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func configuredFormSchema(metadata map[string]any) FormSchema {
	raw, ok := metadata["form_schema"]
	if !ok {
		return FormSchema{}
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return FormSchema{}
	}
	var schema FormSchema
	if err := json.Unmarshal(buf, &schema); err != nil {
		return FormSchema{}
	}
	return schema
}

func extractAgentOriginalOutput(metadata map[string]any) map[string]any {
	reviewRaw, ok := metadata["agent_review"]
	if !ok {
		return map[string]any{}
	}
	reviewMap, ok := reviewRaw.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	orig, ok := reviewMap["original_output"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return orig
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
	return nil, engine.ErrStepAwaitingReview
}
