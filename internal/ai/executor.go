package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/llm"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

const maxValidationAttempts = 3

type LLMManager interface {
	ResolveModel(tenantID uuid.UUID, hint string) string
	Chat(ctx context.Context, tenantID uuid.UUID, req llm.ChatRequest) (llm.ChatResponse, error)
}

type CaseStore interface {
	GetCaseContext(ctx context.Context, tenantID uuid.UUID, caseID uuid.UUID) (CaseRecord, error)
}

type TaskStore interface {
	CreateReviewTask(ctx context.Context, req ReviewTaskRequest) error
}

type CaseRecord struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	CaseType string
	Data     map[string]any
}

type ReviewTaskRequest struct {
	CaseID       uuid.UUID
	StepID       string
	ComponentID  string
	Output       json.RawMessage
	Confidence   *float64
	AssignToRole string
}

type ComponentExecutor struct {
	llm       LLMManager
	caseStore CaseStore
	taskStore TaskStore
	registry  *ComponentRegistry
}

func NewComponentExecutor(llm LLMManager, caseStore CaseStore, taskStore TaskStore, registry *ComponentRegistry) *ComponentExecutor {
	return &ComponentExecutor{llm: llm, caseStore: caseStore, taskStore: taskStore, registry: registry}
}

type ComponentExecRequest struct {
	TenantID     uuid.UUID         `json:"tenant_id"`
	CaseID       uuid.UUID         `json:"case_id"`
	StepID       string            `json:"step_id"`
	ComponentID  string            `json:"component_id"`
	InputPaths   map[string]string `json:"input_paths"`
	OutputPath   string            `json:"output_path"`
	ConfigValues map[string]string `json:"config_values"`
}

type ComponentExecResult struct {
	Output            json.RawMessage `json:"output"`
	Confidence        *float64        `json:"confidence,omitempty"`
	Status            string          `json:"status"`
	MergePatch        json.RawMessage `json:"merge_patch,omitempty"`
	Model             string          `json:"model"`
	InputTokens       int             `json:"input_tokens"`
	OutputTokens      int             `json:"output_tokens"`
	TotalTokens       int             `json:"total_tokens"`
	ConfidenceWarning bool            `json:"confidence_warning,omitempty"`
	Event             json.RawMessage `json:"event,omitempty"`
}

func (ce *ComponentExecutor) Execute(ctx context.Context, req ComponentExecRequest) (ComponentExecResult, error) {
	if ce == nil || ce.registry == nil {
		return ComponentExecResult{}, fmt.Errorf("component executor not configured")
	}
	if ce.llm == nil {
		return ComponentExecResult{}, fmt.Errorf("llm adapter manager not configured")
	}
	if ce.caseStore == nil {
		return ComponentExecResult{}, fmt.Errorf("case store not configured")
	}
	def, err := ce.registry.Get(ctx, req.TenantID, req.ComponentID)
	if err != nil {
		return ComponentExecResult{}, fmt.Errorf("resolve component %q: %w", req.ComponentID, err)
	}
	caseCtx, err := ce.caseStore.GetCaseContext(ctx, req.TenantID, req.CaseID)
	if err != nil {
		return ComponentExecResult{}, fmt.Errorf("load case context: %w", err)
	}

	resolvedInput := resolveInputValues(caseCtx.Data, req.InputPaths)
	renderedPrompt, err := RenderPrompt(def.UserPromptTmpl, PromptData{
		Input:  resolvedInput,
		Config: req.ConfigValues,
		Case: CaseContext{
			ID:   caseCtx.ID.String(),
			Type: caseCtx.CaseType,
		},
	})
	if err != nil {
		return ComponentExecResult{}, err
	}

	messages := []llm.Message{{Role: "user", Content: renderedPrompt}}
	modelHint := resolveModelHint(def.ModelHints)
	model := strings.TrimSpace(ce.llm.ResolveModel(req.TenantID, modelHint))

	started := time.Now()
	rawResponses := make([]string, 0, maxValidationAttempts)
	validationAttempts := make([][]ValidationError, 0, maxValidationAttempts)
	var (
		response llm.ChatResponse
		output   json.RawMessage
		parsed   map[string]any
	)

	for attempt := 1; attempt <= maxValidationAttempts; attempt++ {
		response, err = ce.llm.Chat(ctx, req.TenantID, llm.ChatRequest{
			SystemPrompt: def.SystemPrompt,
			Messages:     messages,
			Model:        model,
			MaxTokens:    def.ModelHints.MaxTokens,
			JSONMode:     true,
			Purpose:      "ai_component",
		})
		if err != nil {
			return ComponentExecResult{}, fmt.Errorf("component llm chat attempt %d: %w", attempt, err)
		}
		if strings.TrimSpace(response.Model) != "" {
			model = response.Model
		}
		rawResponses = append(rawResponses, response.Content)
		output = json.RawMessage(strings.TrimSpace(response.Content))
		if !json.Valid(output) {
			validationErr := []ValidationError{{Path: "$", Message: "invalid json from model"}}
			validationAttempts = append(validationAttempts, validationErr)
			if attempt == maxValidationAttempts {
				return ComponentExecResult{}, buildValidationFailure(validationAttempts, rawResponses)
			}
			messages = append(messages,
				llm.Message{Role: "assistant", Content: response.Content},
				llm.Message{Role: "user", Content: correctivePrompt(validationErr)},
			)
			continue
		}
		validationErr := ValidateOutput(output, def.OutputSchema)
		validationAttempts = append(validationAttempts, validationErr)
		if len(validationErr) == 0 {
			if err := json.Unmarshal(output, &parsed); err != nil {
				return ComponentExecResult{}, fmt.Errorf("decode validated output: %w", err)
			}
			break
		}
		if attempt == maxValidationAttempts {
			return ComponentExecResult{}, buildValidationFailure(validationAttempts, rawResponses)
		}
		messages = append(messages,
			llm.Message{Role: "assistant", Content: response.Content},
			llm.Message{Role: "user", Content: correctivePrompt(validationErr)},
		)
	}

	status := "accepted"
	confidence, hasConfidence := extractConfidence(def.Confidence, parsed)
	warning := false

	if def.Confidence != nil && hasConfidence {
		switch {
		case confidence >= def.Confidence.AutoAcceptAbove:
			status = "accepted"
		case confidence < def.Confidence.EscalateBelow:
			status = "escalated"
		default:
			status = "warning"
			warning = true
		}
	}

	var patch json.RawMessage
	if status != "escalated" {
		patchMap := map[string]any{}
		setDotPath(patchMap, req.OutputPath, parsed)
		patchRaw, mErr := json.Marshal(patchMap)
		if mErr != nil {
			return ComponentExecResult{}, fmt.Errorf("marshal output patch: %w", mErr)
		}
		patch = patchRaw
	} else if ce.taskStore != nil {
		var confPtr *float64
		if hasConfidence {
			c := confidence
			confPtr = &c
		}
		if err := ce.taskStore.CreateReviewTask(ctx, ReviewTaskRequest{
			CaseID:       req.CaseID,
			StepID:       req.StepID,
			ComponentID:  req.ComponentID,
			Output:       output,
			Confidence:   confPtr,
			AssignToRole: "case_worker",
		}); err != nil {
			return ComponentExecResult{}, fmt.Errorf("create ai review task: %w", err)
		}
	}

	eventData := map[string]any{
		"type":               "ai_component_execution",
		"component_id":       req.ComponentID,
		"model":              model,
		"input_values":       resolvedInput,
		"output":             parsed,
		"status":             status,
		"confidence_warning": warning,
		"duration_ms":        time.Since(started).Milliseconds(),
		"tokens": map[string]any{
			"input":  response.InputTokens,
			"output": response.OutputTokens,
			"total":  response.TotalTokens,
		},
		"system_prompt":        def.SystemPrompt,
		"rendered_user_prompt": renderedPrompt,
		"raw_llm_responses":    rawResponses,
		"validation_attempts":  validationAttempts,
	}
	if hasConfidence {
		eventData["confidence"] = confidence
	}
	eventRaw, _ := json.Marshal(eventData)

	result := ComponentExecResult{
		Output:            output,
		Status:            status,
		MergePatch:        patch,
		Model:             model,
		InputTokens:       response.InputTokens,
		OutputTokens:      response.OutputTokens,
		TotalTokens:       response.TotalTokens,
		ConfidenceWarning: warning,
		Event:             eventRaw,
	}
	if hasConfidence {
		result.Confidence = &confidence
	}
	return result, nil
}

type PostgresCaseStore struct {
	db *sql.DB
}

func NewPostgresCaseStore(db *sql.DB) *PostgresCaseStore {
	return &PostgresCaseStore{db: db}
}

func (s *PostgresCaseStore) GetCaseContext(ctx context.Context, tenantID uuid.UUID, caseID uuid.UUID) (CaseRecord, error) {
	if s == nil || s.db == nil {
		return CaseRecord{}, fmt.Errorf("case store not configured")
	}
	var (
		record  CaseRecord
		rawData []byte
	)
	err := s.db.QueryRowContext(ctx, `
SELECT c.id, c.tenant_id, ct.name, c.data
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1 AND c.id = $2
`, tenantID, caseID).Scan(&record.ID, &record.TenantID, &record.CaseType, &rawData)
	if err != nil {
		return CaseRecord{}, err
	}
	if err := json.Unmarshal(rawData, &record.Data); err != nil {
		return CaseRecord{}, fmt.Errorf("decode case data: %w", err)
	}
	return record, nil
}

type TaskServiceAdapter struct {
	service interface {
		CreateTaskFromActivation(ctx context.Context, caseID uuid.UUID, stepID string, cfg tasks.AssignmentConfig) error
	}
}

func NewTaskServiceAdapter(svc interface {
	CreateTaskFromActivation(ctx context.Context, caseID uuid.UUID, stepID string, cfg tasks.AssignmentConfig) error
}) *TaskServiceAdapter {
	return &TaskServiceAdapter{service: svc}
}

func (a *TaskServiceAdapter) CreateReviewTask(ctx context.Context, req ReviewTaskRequest) error {
	if a == nil || a.service == nil {
		return fmt.Errorf("task service not configured")
	}
	assignToRole := strings.TrimSpace(req.AssignToRole)
	if assignToRole == "" {
		assignToRole = "case_worker"
	}
	metadata := map[string]any{
		"ai_component_review": map[string]any{
			"component_id": req.ComponentID,
			"output":       json.RawMessage(req.Output),
		},
	}
	if req.Confidence != nil {
		metadata["ai_component_review"].(map[string]any)["confidence"] = *req.Confidence
	}
	cfg := tasks.AssignmentConfig{
		AssignToRole: assignToRole,
		Form:         "ai_component_review",
		Outcomes:     []string{"accept", "modify", "reject"},
		Metadata:     metadata,
	}
	return a.service.CreateTaskFromActivation(ctx, req.CaseID, req.StepID, cfg)
}

func resolveInputValues(caseData map[string]any, inputPaths map[string]string) map[string]string {
	out := map[string]string{}
	for name, path := range inputPaths {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		value, ok := lookupDotPath(caseData, path)
		if !ok {
			out[name] = ""
			continue
		}
		out[name] = stringifyValue(value)
	}
	return out
}

func resolveModelHint(hints ModelHints) string {
	if hints.RequiresVision {
		return "vision"
	}
	size := strings.TrimSpace(strings.ToLower(hints.PreferredSize))
	if size == "small" || size == "medium" || size == "large" {
		return size
	}
	return "medium"
}

func correctivePrompt(errors []ValidationError) string {
	if len(errors) == 0 {
		return "Your previous response was invalid. Return valid JSON that matches the required schema exactly."
	}
	var b strings.Builder
	b.WriteString("Your previous response did not match the output schema. Fix these errors and return only valid JSON:\n")
	for _, v := range errors {
		b.WriteString("- ")
		b.WriteString(v.Path)
		b.WriteString(": ")
		b.WriteString(v.Message)
		b.WriteString("\n")
	}
	return b.String()
}

func buildValidationFailure(attempts [][]ValidationError, rawResponses []string) error {
	last := attempts[len(attempts)-1]
	parts := make([]string, 0, len(last))
	for _, v := range last {
		parts = append(parts, v.Path+": "+v.Message)
	}
	return fmt.Errorf("component output validation failed after %d attempts: %s; raw_responses=%q", len(attempts), strings.Join(parts, "; "), rawResponses)
}

func extractConfidence(cfg *ConfidenceConfig, output map[string]any) (float64, bool) {
	if cfg == nil {
		return 0, false
	}
	value, ok := lookupDotPath(output, cfg.FieldPath)
	if !ok {
		return 0, false
	}
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func lookupDotPath(root map[string]any, path string) (any, bool) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, false
	}
	parts := strings.Split(trimmed, ".")
	var current any = root
	for _, part := range parts {
		nextMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := nextMap[part]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func setDotPath(root map[string]any, path string, value any) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		root["result"] = value
		return
	}
	parts := strings.Split(trimmed, ".")
	current := root
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

func stringifyValue(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(raw)
	}
}
