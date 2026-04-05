package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/rag"
	"github.com/neural-chilli/aceryx/internal/rbac"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

type CaseStore interface {
	CreateCaseMCP(ctx context.Context, tenantID, userID uuid.UUID, caseType string, data map[string]any, triggerWorkflow bool) (uuid.UUID, string, error)
	GetCaseMCP(ctx context.Context, tenantID, caseID uuid.UUID) (CaseView, error)
	UpdateCaseMCP(ctx context.Context, tenantID, userID, caseID uuid.UUID, data map[string]any) error
	SearchCasesMCP(ctx context.Context, tenantID uuid.UUID, in CaseSearchInput) ([]CaseSearchResult, int, error)
}

type TaskStore interface {
	ListTasksMCP(ctx context.Context, tenantID uuid.UUID, in TaskListInput) ([]TaskSummary, error)
	GetTaskMCP(ctx context.Context, tenantID uuid.UUID, taskID string) (TaskDetailView, error)
	CompleteTaskMCP(ctx context.Context, tenantID, userID uuid.UUID, in TaskCompleteInput) (TaskCompleteResult, error)
}

type WorkflowEngine interface {
	GetStatus(ctx context.Context, tenantID, caseID uuid.UUID) (WorkflowStatusView, error)
}

type SearchService interface {
	Search(ctx context.Context, req rag.SearchRequest) (rag.SearchResponse, error)
}

type KBStore interface {
	ListKnowledgeBases(ctx context.Context, tenantID uuid.UUID) ([]rag.KnowledgeBase, error)
}

type CaseTypeStore interface {
	ListCaseTypes(ctx context.Context, tenantID uuid.UUID) ([]cases.CaseType, error)
}

type CaseView struct {
	CaseID     uuid.UUID
	Status     string
	Data       map[string]any
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Timeline   []cases.CaseEvent
	CaseType   string
	CaseNumber string
}

type CaseSearchInput struct {
	CaseType string
	Status   string
	Filters  map[string]any
	Since    *time.Time
	Limit    int
}

type CaseSearchResult struct {
	CaseID     uuid.UUID
	Status     string
	Summary    string
	CreatedAt  time.Time
	CaseType   string
	CaseNumber string
}

type TaskListInput struct {
	Role   string
	CaseID *uuid.UUID
	Status string
	Limit  int
}

type TaskSummary struct {
	TaskID      string
	CaseID      uuid.UUID
	Type        string
	Status      string
	AssignedTo  *uuid.UUID
	SLA         *time.Time
	CreatedAt   time.Time
	Role        string
	CaseNumber  string
	CaseType    string
	DisplayName string
}

type TaskDetailView struct {
	TaskID          string
	CaseID          uuid.UUID
	Type            string
	Status          string
	FormSchema      map[string]any
	CaseData        map[string]any
	ReasoningTrace  any
	AvailableAction []string
}

type TaskCompleteInput struct {
	TaskID   string
	Decision string
	FormData map[string]any
	Notes    string
}

type TaskCompleteResult struct {
	TaskID string
	CaseID uuid.UUID
	Status string
}

type WorkflowStatusView struct {
	CaseID         uuid.UUID
	WorkflowID     uuid.UUID
	Status         string
	CurrentStep    string
	CompletedSteps int
	PendingTasks   int
	ProgressPct    float64
}

type CompositeStore struct {
	Cases     *cases.CaseService
	CaseTypes *cases.CaseTypeService
	Tasks     *tasks.TaskService
	Engine    *engine.Engine
	KBs       rag.KnowledgeBaseStore
	SearchSvc *rag.SearchService
	DB        *sql.DB
}

func NewCompositeStore(db *sql.DB, eng *engine.Engine) *CompositeStore {
	return &CompositeStore{
		Cases:     cases.NewCaseService(db, eng),
		CaseTypes: cases.NewCaseTypeService(db),
		Tasks:     tasks.NewTaskService(db, eng, nil),
		Engine:    eng,
		KBs:       nil,
		SearchSvc: nil,
		DB:        db,
	}
}

func (s *CompositeStore) CreateCaseMCP(ctx context.Context, tenantID, userID uuid.UUID, caseType string, data map[string]any, _ bool) (uuid.UUID, string, error) {
	created, vErrs, err := s.Cases.CreateCase(ctx, tenantID, userID, cases.CreateCaseRequest{CaseType: strings.TrimSpace(caseType), Data: data})
	if err != nil {
		return uuid.Nil, "", err
	}
	if len(vErrs) > 0 {
		return uuid.Nil, "", fmt.Errorf("validation_failed: %s", vErrs[0].Message)
	}
	return created.ID, created.Status, nil
}

func (s *CompositeStore) GetCaseMCP(ctx context.Context, tenantID, caseID uuid.UUID) (CaseView, error) {
	c, err := s.Cases.GetCase(ctx, tenantID, caseID)
	if err != nil {
		return CaseView{}, err
	}
	return CaseView{CaseID: c.ID, Status: c.Status, Data: c.Data, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Timeline: c.Events, CaseType: c.CaseType, CaseNumber: c.CaseNumber}, nil
}

func (s *CompositeStore) UpdateCaseMCP(ctx context.Context, tenantID, userID, caseID uuid.UUID, data map[string]any) error {
	current, err := s.Cases.GetCase(ctx, tenantID, caseID)
	if err != nil {
		return err
	}
	_, vErrs, err := s.Cases.UpdateCaseData(ctx, tenantID, caseID, userID, data, current.Version)
	if err != nil {
		return err
	}
	if len(vErrs) > 0 {
		return fmt.Errorf("validation_failed: %s", vErrs[0].Message)
	}
	return nil
}

func (s *CompositeStore) SearchCasesMCP(ctx context.Context, tenantID uuid.UUID, in CaseSearchInput) ([]CaseSearchResult, int, error) {
	if in.Limit <= 0 {
		in.Limit = 20
	}
	items, err := s.Cases.ListCases(ctx, tenantID, cases.ListCasesFilter{CaseType: strings.TrimSpace(in.CaseType), Statuses: compactCSV(in.Status), PerPage: in.Limit, Page: 1})
	if err != nil {
		return nil, 0, err
	}
	out := make([]CaseSearchResult, 0, len(items))
	for _, item := range items {
		if in.Since != nil && item.CreatedAt.Before(*in.Since) {
			continue
		}
		out = append(out, CaseSearchResult{CaseID: item.ID, Status: item.Status, Summary: item.CaseNumber + " " + item.CaseType, CreatedAt: item.CreatedAt, CaseType: item.CaseType, CaseNumber: item.CaseNumber})
	}
	return out, len(out), nil
}

func compactCSV(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return []string{v}
}

func (s *CompositeStore) ListTasksMCP(ctx context.Context, tenantID uuid.UUID, in TaskListInput) ([]TaskSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT cs.case_id, cs.step_id, cs.state, cs.assigned_to, cs.sla_deadline, cs.started_at,
       COALESCE(cs.metadata->>'role', ''), COALESCE(cs.metadata->>'label', cs.step_id),
       c.case_number, ct.name
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1
  AND cs.state = 'active'
ORDER BY cs.started_at DESC
LIMIT $2
`, tenantID, max(1, in.Limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]TaskSummary, 0)
	for rows.Next() {
		var summary TaskSummary
		var stepID string
		var state string
		var role string
		var display string
		var started sql.NullTime
		if err := rows.Scan(&summary.CaseID, &stepID, &state, &summary.AssignedTo, &summary.SLA, &started, &role, &display, &summary.CaseNumber, &summary.CaseType); err != nil {
			return nil, err
		}
		summary.TaskID = summary.CaseID.String() + ":" + stepID
		summary.Type = "human_task"
		summary.Status = state
		summary.Role = role
		summary.DisplayName = display
		if started.Valid {
			summary.CreatedAt = started.Time
		}
		if in.Role != "" && role != in.Role {
			continue
		}
		if in.CaseID != nil && summary.CaseID != *in.CaseID {
			continue
		}
		if in.Status != "" && summary.Status != in.Status {
			continue
		}
		out = append(out, summary)
	}
	return out, rows.Err()
}

func parseTaskID(taskID string) (uuid.UUID, string, error) {
	parts := strings.Split(strings.TrimSpace(taskID), ":")
	if len(parts) != 2 {
		return uuid.Nil, "", fmt.Errorf("task_id must be <case_id>:<step_id>")
	}
	cid, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("invalid case id in task_id: %w", err)
	}
	if strings.TrimSpace(parts[1]) == "" {
		return uuid.Nil, "", fmt.Errorf("invalid step id in task_id")
	}
	return cid, parts[1], nil
}

func (s *CompositeStore) GetTaskMCP(ctx context.Context, tenantID uuid.UUID, taskID string) (TaskDetailView, error) {
	caseID, stepID, err := parseTaskID(taskID)
	if err != nil {
		return TaskDetailView{}, err
	}
	detail, err := s.Tasks.GetTask(ctx, tenantID, caseID, stepID)
	if err != nil {
		return TaskDetailView{}, err
	}
	formSchema := map[string]any{}
	raw, _ := json.Marshal(detail.FormSchema)
	_ = json.Unmarshal(raw, &formSchema)
	return TaskDetailView{
		TaskID:          taskID,
		CaseID:          caseID,
		Type:            detail.StepType,
		Status:          detail.StepState,
		FormSchema:      formSchema,
		CaseData:        detail.CaseData,
		ReasoningTrace:  detail.Metadata["reasoning_trace"],
		AvailableAction: detail.AvailableActions,
	}, nil
}

func (s *CompositeStore) CompleteTaskMCP(ctx context.Context, tenantID, userID uuid.UUID, in TaskCompleteInput) (TaskCompleteResult, error) {
	caseID, stepID, err := parseTaskID(in.TaskID)
	if err != nil {
		return TaskCompleteResult{}, err
	}
	if err := s.Tasks.CompleteTask(ctx, tenantID, userID, caseID, stepID, tasks.CompleteTaskRequest{Outcome: in.Decision, Data: in.FormData}); err != nil {
		return TaskCompleteResult{}, err
	}
	return TaskCompleteResult{TaskID: in.TaskID, CaseID: caseID, Status: "completed"}, nil
}

func (s *CompositeStore) GetStatus(ctx context.Context, tenantID, caseID uuid.UUID) (WorkflowStatusView, error) {
	if s.Engine == nil {
		return WorkflowStatusView{}, fmt.Errorf("workflow engine not configured")
	}
	status, err := s.Engine.GetStatus(ctx, tenantID, caseID)
	if err != nil {
		return WorkflowStatusView{}, err
	}
	return WorkflowStatusView{
		CaseID:         status.CaseID,
		WorkflowID:     status.WorkflowID,
		Status:         status.Status,
		CurrentStep:    status.CurrentStep,
		CompletedSteps: status.CompletedSteps,
		PendingTasks:   status.PendingTasks,
		ProgressPct:    status.ProgressPct,
	}, nil
}

func (s *CompositeStore) SearchRAG(ctx context.Context, req rag.SearchRequest) (rag.SearchResponse, error) {
	if s.SearchSvc == nil {
		return rag.SearchResponse{}, fmt.Errorf("search service not configured")
	}
	return s.SearchSvc.Search(ctx, req)
}

func (s *CompositeStore) ListKnowledgeBases(ctx context.Context, tenantID uuid.UUID) ([]rag.KnowledgeBase, error) {
	if s.KBs == nil {
		return nil, fmt.Errorf("knowledge base store not configured")
	}
	return s.KBs.List(ctx, tenantID)
}

func (s *CompositeStore) ListCaseTypes(ctx context.Context, tenantID uuid.UUID) ([]cases.CaseType, error) {
	return s.CaseTypes.ListCaseTypes(ctx, tenantID, false)
}

func (s *CompositeStore) ValidateRolePermission(ctx context.Context, principalID uuid.UUID, permission string) error {
	return rbac.Authorize(ctx, s.DB, principalID, permission)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
