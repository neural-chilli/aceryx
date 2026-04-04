package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
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
	auditSvc *audit.Service
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
		db:       db,
		engine:   eng,
		notify:   notify,
		auditSvc: audit.NewService(db),
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

func (s *TaskService) SetAuditService(auditSvc *audit.Service) {
	if auditSvc == nil {
		return
	}
	s.auditSvc = auditSvc
}

func (s *TaskService) CreateTaskFromActivation(ctx context.Context, caseID uuid.UUID, stepID string, cfg AssignmentConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin task creation tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()

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
	if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "task", actor, "system", "created", map[string]any{"assigned_to": assignedTo, "role": cfg.AssignToRole, "sla_hours": cfg.SLAHours}); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cases SET updated_at = now() WHERE id = $1`, caseID); err != nil {
		return fmt.Errorf("touch case timestamp task create: %w", err)
	}

	if err := s.auditSvc.CommitTx(tx); err != nil {
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
		caseNumber, _ := s.lookupCaseNumber(ctx, tenantID, caseID)
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
