package cases

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ValidationError struct {
	Field   string      `json:"field"`
	Rule    string      `json:"rule"`
	Message string      `json:"message"`
	Value   interface{} `json:"value,omitempty"`
}

type CaseTypeSchema struct {
	Fields map[string]SchemaField `json:"fields"`
}

type SchemaField struct {
	Type       string                 `json:"type"`
	Required   bool                   `json:"required,omitempty"`
	Pattern    string                 `json:"pattern,omitempty"`
	Min        *float64               `json:"min,omitempty"`
	Max        *float64               `json:"max,omitempty"`
	Enum       []interface{}          `json:"enum,omitempty"`
	MinLength  *int                   `json:"minLength,omitempty"`
	MaxLength  *int                   `json:"maxLength,omitempty"`
	Source     string                 `json:"source,omitempty"`
	Properties map[string]SchemaField `json:"properties,omitempty"`
}

type CaseType struct {
	ID        uuid.UUID      `json:"id"`
	TenantID  uuid.UUID      `json:"tenant_id"`
	Name      string         `json:"name"`
	Version   int            `json:"version"`
	Schema    CaseTypeSchema `json:"schema"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	CreatedBy uuid.UUID      `json:"created_by"`
}

type Case struct {
	ID              uuid.UUID              `json:"id"`
	TenantID        uuid.UUID              `json:"tenant_id"`
	CaseTypeID      uuid.UUID              `json:"case_type_id"`
	CaseType        string                 `json:"case_type"`
	CaseNumber      string                 `json:"case_number"`
	Status          string                 `json:"status"`
	Data            map[string]interface{} `json:"data"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	CreatedBy       uuid.UUID              `json:"created_by"`
	AssignedTo      *uuid.UUID             `json:"assigned_to,omitempty"`
	DueAt           *time.Time             `json:"due_at,omitempty"`
	Priority        int                    `json:"priority"`
	Version         int                    `json:"version"`
	WorkflowID      uuid.UUID              `json:"workflow_id"`
	WorkflowVersion int                    `json:"workflow_version"`
	Steps           []CaseStep             `json:"steps,omitempty"`
	Events          []CaseEvent            `json:"events,omitempty"`
	Documents       []CaseDocument         `json:"documents,omitempty"`
}

type CaseStep struct {
	ID          uuid.UUID       `json:"id"`
	StepID      string          `json:"step_id"`
	State       string          `json:"state"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	Events      json.RawMessage `json:"events,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
	AssignedTo  *uuid.UUID      `json:"assigned_to,omitempty"`
	SLADeadline *time.Time      `json:"sla_deadline,omitempty"`
	RetryCount  int             `json:"retry_count"`
	DraftData   json.RawMessage `json:"draft_data,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type CaseEvent struct {
	ID            uuid.UUID              `json:"id"`
	StepID        string                 `json:"step_id,omitempty"`
	EventType     string                 `json:"event_type"`
	ActorID       uuid.UUID              `json:"actor_id"`
	ActorType     string                 `json:"actor_type"`
	Action        string                 `json:"action"`
	Data          map[string]interface{} `json:"data,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	PrevEventHash string                 `json:"prev_event_hash"`
	EventHash     string                 `json:"event_hash"`
}

type CaseDocument struct {
	ID         uuid.UUID  `json:"id"`
	Filename   string     `json:"filename"`
	MimeType   string     `json:"mime_type"`
	SizeBytes  int64      `json:"size_bytes"`
	UploadedBy uuid.UUID  `json:"uploaded_by"`
	UploadedAt time.Time  `json:"uploaded_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

type CreateCaseRequest struct {
	CaseType string                 `json:"case_type"`
	Data     map[string]interface{} `json:"data"`
	Priority int                    `json:"priority"`
}

type PatchResult struct {
	Case Case                 `json:"case"`
	Diff map[string]FieldDiff `json:"diff"`
}

type FieldDiff struct {
	Before interface{} `json:"before,omitempty"`
	After  interface{} `json:"after,omitempty"`
}

type ListCasesFilter struct {
	Statuses      []string
	CaseType      string
	AssignedTo    *uuid.UUID
	AssignedMe    bool
	AssignedNone  bool
	Priority      *int
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	DueBefore     *time.Time
	Page          int
	PerPage       int
	SortBy        string
	SortDir       string
}

type SearchFilter struct {
	Query    string
	CaseType string
	Status   string
	Page     int
	PerPage  int
}

type SearchResult struct {
	CaseID      uuid.UUID `json:"case_id"`
	CaseNumber  string    `json:"case_number"`
	CaseType    string    `json:"case_type"`
	Status      string    `json:"status"`
	Headline    string    `json:"headline"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CaseVersion int       `json:"case_version"`
}

type DashboardFilter struct {
	Statuses      []string
	CaseType      string
	AssignedTo    *uuid.UUID
	AssignedMe    bool
	AssignedNone  bool
	OlderThanDays *int
	Priority      *int
	SLAStatus     string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Page          int
	PerPage       int
	SortBy        string
	SortDir       string
}

type DashboardRow struct {
	CaseID      uuid.UUID  `json:"case_id"`
	CaseNumber  string     `json:"case_number"`
	CaseType    string     `json:"case_type"`
	Status      string     `json:"status"`
	AssignedTo  *uuid.UUID `json:"assigned_to,omitempty"`
	Priority    int        `json:"priority"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	SLAStatus   string     `json:"sla_status"`
	CurrentStep string     `json:"current_step,omitempty"`
}

type CasesSummaryRow struct {
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
	Opened         int       `json:"opened"`
	Closed         int       `json:"closed"`
	Cancelled      int       `json:"cancelled"`
	AvgDaysToClose float64   `json:"avg_days_to_close"`
}

type AgeingBracket struct {
	Label   string      `json:"label"`
	Count   int         `json:"count"`
	CaseIDs []uuid.UUID `json:"case_ids"`
}

type SLAComplianceRow struct {
	PeriodStart        time.Time `json:"period_start"`
	TotalTasks         int       `json:"total_tasks"`
	CompletedWithinSLA int       `json:"completed_within_sla"`
	ComplianceRate     float64   `json:"compliance_rate"`
}

type StageRow struct {
	Stage string `json:"stage"`
	Count int    `json:"count"`
}

type WorkloadRow struct {
	PrincipalID uuid.UUID `json:"principal_id"`
	Name        string    `json:"name"`
	ActiveTasks int       `json:"active_tasks"`
	BreachedSLA int       `json:"breached_sla"`
}

type DecisionRow struct {
	PeriodStart      time.Time `json:"period_start"`
	AgentDecisions   int       `json:"agent_decisions"`
	HumanDecisions   int       `json:"human_decisions"`
	AgentEscalations int       `json:"agent_escalations"`
}
