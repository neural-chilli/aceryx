package agents

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/observability"
)

type AgentExecutor struct {
	db              *sql.DB
	tasks           TaskCreator
	prompts         *PromptTemplateService
	auditSvc        *audit.Service
	llm             *LLMClient
	defaultModel    string
	contextTimeout  time.Duration
	sourceTimeout   time.Duration
	contextMaxBytes int
	llmTimeout      time.Duration
}

func NewAgentExecutor(cfg ExecutorConfig) *AgentExecutor {
	ctxTimeout := cfg.ContextTimeout
	if ctxTimeout <= 0 {
		ctxTimeout = defaultContextTimeout
	}
	srcTimeout := cfg.SourceTimeout
	if srcTimeout <= 0 {
		srcTimeout = defaultSourceTimeout
	}
	ctxMaxBytes := cfg.ContextMaxBytes
	if ctxMaxBytes <= 0 {
		ctxMaxBytes = defaultContextMaxBytes
	}
	llmTimeout := cfg.LLMTimeout
	if llmTimeout <= 0 {
		llmTimeout = defaultLLMTimeout
	}
	llm := cfg.LLMClient
	if llm == nil {
		llm = NewLLMClientFromEnv(llmTimeout)
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" && llm != nil {
		model = llm.model
	}
	return &AgentExecutor{
		db:              cfg.DB,
		tasks:           cfg.TaskCreator,
		prompts:         NewPromptTemplateService(cfg.DB),
		auditSvc:        resolveAuditService(cfg.DB, cfg.AuditService),
		llm:             llm,
		defaultModel:    model,
		contextTimeout:  ctxTimeout,
		sourceTimeout:   srcTimeout,
		contextMaxBytes: ctxMaxBytes,
		llmTimeout:      llmTimeout,
	}
}

func resolveAuditService(db *sql.DB, svc *audit.Service) *audit.Service {
	if svc != nil {
		return svc
	}
	return audit.NewService(db)
}

func (a *AgentExecutor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, raw json.RawMessage) (*engine.StepResult, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("agent executor not configured")
	}

	cfg := StepConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse agent step config: %w", err)
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultValidationMaxAttempt
	}

	tenantID, caseNumber, caseTypeName, caseData, stepResults, err := a.loadCaseAndSteps(ctx, caseID)
	if err != nil {
		return nil, err
	}

	ctxStage, cancelCtx := context.WithTimeout(ctx, a.contextTimeout)
	defer cancelCtx()
	assembled, err := a.ResolveContext(ctxStage, tenantID, caseID, caseData, stepResults, cfg.Context)
	if err != nil {
		return nil, fmt.Errorf("assemble agent context: %w", err)
	}

	tpl, err := a.prompts.resolveTemplate(ctx, tenantID, cfg.PromptTemplate, cfg.PromptVersion)
	if err != nil {
		return nil, err
	}

	model := a.defaultModel
	if strings.TrimSpace(cfg.Model) != "" && cfg.Model != "default" {
		model = strings.TrimSpace(cfg.Model)
	}
	if model == "" && a.llm != nil {
		model = a.llm.model
	}
	if model == "" {
		return nil, fmt.Errorf("llm model not configured")
	}
	observability.AgentInvocationsTotal.WithLabelValues(tenantID.String(), model).Inc()

	outputSchemaAny := map[string]any{}
	for k, v := range cfg.OutputSchema {
		outputSchemaAny[k] = v
	}
	promptData := map[string]any{
		"case":          assembled.Case,
		"steps":         assembled.Steps,
		"knowledge":     assembled.Knowledge,
		"vault":         assembled.Vault,
		"output_schema": outputSchemaAny,
		"case_metadata": map[string]any{
			"case_number": caseNumber,
			"case_type":   caseTypeName,
		},
		"now": time.Now().UTC().Format(time.RFC3339),
	}

	renderedPrompt, err := renderPromptTemplate(tpl.Template, promptData)
	if err != nil {
		return nil, err
	}
	promptHash := sha256.Sum256([]byte(renderedPrompt))

	resultObj, usage, latencyMs, err := a.invokeWithValidationRetry(ctx, model, renderedPrompt, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "agent execution failed",
			append(observability.RequestAttrs(ctx),
				"case_id", caseID.String(),
				"step_id", stepID,
				"tenant_id", tenantID.String(),
				"model", model,
				"error", err,
			)...,
		)
		return nil, err
	}
	observability.AgentDurationSeconds.WithLabelValues(tenantID.String()).Observe(float64(latencyMs) / 1000.0)
	observability.AgentTokensTotal.WithLabelValues(tenantID.String(), model, "input").Add(float64(usage.InputTokens))
	observability.AgentTokensTotal.WithLabelValues(tenantID.String(), model, "output").Add(float64(usage.OutputTokens))

	confidence, _ := asFloat(resultObj["confidence"])
	observability.AgentConfidenceScore.WithLabelValues(tenantID.String()).Observe(confidence)
	if confidence < cfg.ConfidenceThreshold && strings.EqualFold(cfg.OnLowConfidence, "escalate_to_human") {
		observability.AgentEscalationsTotal.WithLabelValues(tenantID.String()).Inc()
		if err := a.createHumanReviewTask(ctx, caseID, stepID, cfg, resultObj, confidence); err != nil {
			return nil, err
		}
		return nil, engine.ErrStepAwaitingReview
	}

	slog.InfoContext(ctx, "agent step completed",
		append(observability.RequestAttrs(ctx),
			"case_id", caseID.String(),
			"step_id", stepID,
			"tenant_id", tenantID.String(),
			"model", model,
			"confidence", confidence,
			"latency_ms", latencyMs,
		)...,
	)

	resultPayload, err := json.Marshal(resultObj)
	if err != nil {
		return nil, fmt.Errorf("marshal agent output: %w", err)
	}

	event := map[string]any{
		"type":                 "llm_call",
		"model":                model,
		"prompt_template":      fmt.Sprintf("%s_v%d", tpl.Name, tpl.Version),
		"tokens":               map[string]any{"input": usage.InputTokens, "output": usage.OutputTokens},
		"latency_ms":           latencyMs,
		"confidence":           confidence,
		"context_snapshot":     assembled.Meta,
		"output":               resultObj,
		"rendered_prompt":      renderedPrompt,
		"rendered_prompt_hash": "sha256:" + hex.EncodeToString(promptHash[:]),
	}
	eventJSON, _ := json.Marshal(event)

	stepResult := &engine.StepResult{
		Output:         resultPayload,
		ExecutionEvent: eventJSON,
		AuditEventType: "agent.completed",
	}
	if cfg.WritesCaseData {
		stepResult.WritesCaseData = true
		if cfg.CaseDataField != "" {
			stepResult.CaseDataPatch, _ = json.Marshal(map[string]any{cfg.CaseDataField: resultObj})
		} else {
			stepResult.CaseDataPatch, _ = json.Marshal(resultObj)
		}
	}
	return stepResult, nil
}
