package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	StatePending   = "pending"
	StateReady     = "ready"
	StateActive    = "active"
	StateCompleted = "completed"
	StateFailed    = "failed"
	StateSkipped   = "skipped"
)

var (
	ErrNotFound            = errors.New("engine: not found")
	ErrCaseDataConflict    = errors.New("engine: case data optimistic lock conflict")
	ErrStepAwaitingReview  = errors.New("engine: step awaiting external human review")
	ErrStepNotActive       = errors.New("engine: step is not active")
	ErrExpressionTooLarge  = errors.New("engine: expression exceeds maximum size")
	ErrExpressionTimedOut  = errors.New("engine: expression evaluation timeout")
	ErrCycleDetectedInAST  = errors.New("engine: cycle detected in workflow AST")
	ErrInvalidJoinStrategy = errors.New("engine: invalid join strategy")
)

type TransitionType string

const (
	TransitionToReady   TransitionType = "to_ready"
	TransitionToActive  TransitionType = "to_active"
	TransitionToSkipped TransitionType = "to_skipped"
)

type TransitionReason string

const (
	ReasonDependenciesSatisfied TransitionReason = "dependencies_satisfied"
	ReasonJoinAnySatisfied      TransitionReason = "join_any_satisfied"
	ReasonGuardFalse            TransitionReason = "guard_false"
	ReasonSkipPropagation       TransitionReason = "skip_propagation"
	ReasonOutcomeRouting        TransitionReason = "outcome_routing"
)

type Transition struct {
	StepID  string
	From    string
	To      string
	Type    TransitionType
	Reason  TransitionReason
	Outcome string
}

type WorkflowAST struct {
	Steps []WorkflowStep `json:"steps"`
}

type WorkflowStep struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	DependsOn   []string               `json:"depends_on"`
	Join        string                 `json:"join,omitempty"`
	Condition   string                 `json:"condition,omitempty"`
	Outcomes    map[string][]string    `json:"outcomes,omitempty"`
	Config      json.RawMessage        `json:"config,omitempty"`
	ErrorPolicy ErrorPolicy            `json:"error_policy,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type StepState struct {
	StepID      string          `json:"step_id"`
	State       string          `json:"state"`
	Result      json.RawMessage `json:"result,omitempty"`
	RetryCount  int             `json:"retry_count"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

type StepResult struct {
	Outcome        string          `json:"outcome,omitempty"`
	Output         json.RawMessage `json:"output,omitempty"`
	WritesCaseData bool            `json:"writes_case_data,omitempty"`
	CaseDataPatch  json.RawMessage `json:"case_data_patch,omitempty"`
	ExecutionEvent json.RawMessage `json:"execution_event,omitempty"`
	AuditEventType string          `json:"audit_event_type,omitempty"`
	Attempts       int             `json:"attempts,omitempty"`
}

// StepExecutor executes one active step.
type StepExecutor interface {
	Execute(ctx context.Context, caseID uuid.UUID, stepID string, config json.RawMessage) (*StepResult, error)
}

type ErrorPolicy struct {
	MaxAttempts  int           `json:"max_attempts"`
	Backoff      string        `json:"backoff"`
	InitialDelay time.Duration `json:"initial_delay"`
	MaxDelay     time.Duration `json:"max_delay"`
	OnExhausted  string        `json:"on_exhausted"`
}

type Config struct {
	MaxConcurrentSteps       int
	MaxConcurrentEvaluations int
	SLAInterval              time.Duration
}

type EscalationCallback func(ctx context.Context, task OverdueTask) error

type OverdueTask struct {
	ID          uuid.UUID
	CaseID      uuid.UUID
	StepID      string
	SLADeadline time.Time
	AssignedTo  *uuid.UUID
}

type Engine struct {
	db            *sql.DB
	evaluations   *WorkerPool
	executions    *WorkerPool
	evaluators    ExpressionEvaluator
	executors     map[string]StepExecutor
	escalation    EscalationCallback
	systemActorID uuid.UUID
	mu            sync.RWMutex
	defaultPolicy ErrorPolicy
	slaInterval   time.Duration
}

type ExpressionEvaluator interface {
	EvaluateBool(expr string, context map[string]interface{}) (bool, error)
}

func defaultConfig(cfg Config) Config {
	if cfg.MaxConcurrentSteps <= 0 {
		cfg.MaxConcurrentSteps = 10
	}
	if cfg.MaxConcurrentEvaluations <= 0 {
		cfg.MaxConcurrentEvaluations = 10
	}
	if cfg.SLAInterval <= 0 {
		cfg.SLAInterval = 60 * time.Second
	}
	return cfg
}

func defaultErrorPolicyForStep(stepType string, policy ErrorPolicy) ErrorPolicy {
	if policy.MaxAttempts <= 0 {
		switch stepType {
		case "integration", "agent":
			policy.MaxAttempts = 3
		default:
			policy.MaxAttempts = 1
		}
	}
	if policy.Backoff == "" {
		policy.Backoff = "none"
	}
	if policy.InitialDelay <= 0 {
		policy.InitialDelay = 5 * time.Second
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = 60 * time.Second
	}
	if policy.OnExhausted == "" {
		policy.OnExhausted = "fail"
	}
	return policy
}

func New(db *sql.DB, evaluator ExpressionEvaluator, cfg Config) *Engine {
	cfg = defaultConfig(cfg)
	return &Engine{
		db:            db,
		evaluators:    evaluator,
		executors:     make(map[string]StepExecutor),
		executions:    NewWorkerPool(cfg.MaxConcurrentSteps),
		evaluations:   NewWorkerPool(cfg.MaxConcurrentEvaluations),
		systemActorID: uuid.Nil,
		slaInterval:   cfg.SLAInterval,
		defaultPolicy: ErrorPolicy{MaxAttempts: 1, Backoff: "none", InitialDelay: 5 * time.Second, MaxDelay: 60 * time.Second, OnExhausted: "fail"},
	}
}

func (e *Engine) RegisterExecutor(stepType string, executor StepExecutor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executors[stepType] = executor
}

func (e *Engine) SetEscalationCallback(cb EscalationCallback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.escalation = cb
}

func (e *Engine) SetSystemActorID(actorID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.systemActorID = actorID
}

func (e *Engine) executorFor(stepType string) (StepExecutor, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	exec, ok := e.executors[stepType]
	if !ok {
		return nil, fmt.Errorf("executor not registered for step type %q", stepType)
	}
	return exec, nil
}

func (e *Engine) WorkerPoolStats() (active int, capacity int) {
	if e == nil {
		return 0, 0
	}
	active = 0
	capacity = 0
	if e.executions != nil {
		active += e.executions.Active()
		capacity += e.executions.Capacity()
	}
	if e.evaluations != nil {
		active += e.evaluations.Active()
		capacity += e.evaluations.Capacity()
	}
	return active, capacity
}

func (e *Engine) Wait() {
	if e == nil {
		return
	}
	if e.executions != nil {
		e.executions.Wait()
	}
	if e.evaluations != nil {
		e.evaluations.Wait()
	}
}
