package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/agents"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/expressions"
	"github.com/neural-chilli/aceryx/internal/observability"
	"github.com/neural-chilli/aceryx/internal/tasks"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestAgentsIntegration_FullExecution(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "a006-full")
	caseTypeID := seedAdditionalCaseType(t, ctx, db, tenantID, principalID, "agent_case")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "test-model",
				"choices": []map[string]any{{
					"finish_reason": "stop",
					"message":       map[string]any{"content": `{"score":88,"risk_level":"low","reasoning":"ok","flags":["none"],"confidence":0.95}`},
				}},
				"usage": map[string]any{"prompt_tokens": 50, "completion_tokens": 20},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	promptSvc := agents.NewPromptTemplateService(db)
	_, err := promptSvc.Create(ctx, tenantID, principalID, agents.CreatePromptTemplateRequest{
		Name:     "risk_assessment",
		Template: `Case {{.case.applicant.company_name}} output {{toJSON .output_schema}}`,
	})
	if err != nil {
		t.Fatalf("create prompt template: %v", err)
	}

	stepCfg, _ := json.Marshal(map[string]any{
		"prompt_template":      "risk_assessment_v1",
		"model":                "test-model",
		"context":              []map[string]any{{"source": "case", "fields": []string{"applicant.company_name"}}},
		"output_schema":        map[string]any{"score": map[string]any{"type": "number", "min": 0, "max": 100}, "risk_level": map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}}, "reasoning": map[string]any{"type": "text"}, "flags": map[string]any{"type": "array", "items": "string"}, "confidence": map[string]any{"type": "number", "min": 0, "max": 1}},
		"confidence_threshold": 0.8,
		"on_low_confidence":    "escalate_to_human",
		"max_attempts":         2,
	})

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "agent", Type: "agent", Config: stepCfg}}}
	workflowID, _ := seedPublishedWorkflow(t, ctx, db, tenantID, principalID, "agent_case", ast)

	var caseID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, 'AGENT-000001', 'open', '{"applicant":{"company_name":"Acme"}}'::jsonb, $3, $4, 1)
RETURNING id
`, tenantID, caseTypeID, principalID, workflowID).Scan(&caseID); err != nil {
		t.Fatalf("insert case: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO case_steps (case_id, step_id, state, events, retry_count) VALUES ($1, 'agent', 'pending', '[]'::jsonb, 0)`, caseID); err != nil {
		t.Fatalf("insert case step: %v", err)
	}

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	agentExec := agents.NewAgentExecutor(agents.ExecutorConfig{DB: db, LLMClient: agents.NewLLMClient(srv.URL, "test-model", "", 2*time.Second)})
	en.RegisterExecutor("agent", agentExec)

	if err := en.EvaluateDAG(ctx, caseID); err != nil {
		t.Fatalf("evaluate dag: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "agent", engine.StateCompleted)

	var eventCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM case_events WHERE case_id = $1 AND event_type = 'agent' AND action = 'completed'`, caseID).Scan(&eventCount); err != nil {
		t.Fatalf("count case events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("expected agent.completed event, got %d", eventCount)
	}
}

func TestAgentsIntegration_PromptTemplateCRUD(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "a006-prompts")
	svc := agents.NewPromptTemplateService(db)

	created, err := svc.Create(ctx, tenantID, principalID, agents.CreatePromptTemplateRequest{
		Name:         "risk_prompt",
		Template:     "v1 {{.case.id}}",
		OutputSchema: map[string]any{"score": map[string]any{"type": "number"}},
	})
	if err != nil {
		t.Fatalf("create prompt template: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1, got %d", created.Version)
	}

	v2, err := svc.CreateVersion(ctx, tenantID, principalID, "risk_prompt", agents.UpdatePromptTemplateRequest{
		Template:     "v2 {{.case.id}}",
		OutputSchema: map[string]any{"score": map[string]any{"type": "number"}, "confidence": map[string]any{"type": "number"}},
	})
	if err != nil {
		t.Fatalf("create prompt template version: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("expected version 2, got %d", v2.Version)
	}

	gotV1, err := svc.GetVersion(ctx, tenantID, "risk_prompt", 1)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if gotV1.Template != "v1 {{.case.id}}" {
		t.Fatalf("unexpected v1 template: %s", gotV1.Template)
	}
}

func TestAgentsIntegration_LowConfidenceEscalatesToHumanReview(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "a006-review")
	caseTypeID := seedAdditionalCaseType(t, ctx, db, tenantID, principalID, "agent_review_case")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "test-model",
				"choices": []map[string]any{{
					"finish_reason": "stop",
					"message":       map[string]any{"content": `{"score":40,"risk_level":"high","reasoning":"needs review","flags":["manual"],"confidence":0.3}`},
				}},
				"usage": map[string]any{"prompt_tokens": 50, "completion_tokens": 20},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	promptSvc := agents.NewPromptTemplateService(db)
	if _, err := promptSvc.Create(ctx, tenantID, principalID, agents.CreatePromptTemplateRequest{Name: "risk_review", Template: `Case {{.case.applicant.company_name}}`}); err != nil {
		t.Fatalf("create prompt template: %v", err)
	}

	stepCfg, _ := json.Marshal(map[string]any{
		"prompt_template":      "risk_review_v1",
		"context":              []map[string]any{{"source": "case", "fields": []string{"applicant.company_name"}}},
		"output_schema":        map[string]any{"score": map[string]any{"type": "number", "min": 0, "max": 100}, "risk_level": map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}}, "reasoning": map[string]any{"type": "text"}, "flags": map[string]any{"type": "array", "items": "string"}, "confidence": map[string]any{"type": "number", "min": 0, "max": 1}},
		"confidence_threshold": 0.8,
		"on_low_confidence":    "escalate_to_human",
		"assign_to_user":       principalID.String(),
	})
	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{ID: "agent", Type: "agent", Config: stepCfg},
		{ID: "after", Type: "rule", DependsOn: []string{"agent"}},
	}}
	workflowID, _ := seedPublishedWorkflow(t, ctx, db, tenantID, principalID, "agent_review_case", ast)

	var caseID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, 'AGENT-000002', 'open', '{"applicant":{"company_name":"Acme"}}'::jsonb, $3, $4, 1)
RETURNING id
`, tenantID, caseTypeID, principalID, workflowID).Scan(&caseID); err != nil {
		t.Fatalf("insert case: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO case_steps (case_id, step_id, state, events, retry_count) VALUES ($1, 'agent', 'pending', '[]'::jsonb, 0)`, caseID); err != nil {
		t.Fatalf("insert case step agent: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO case_steps (case_id, step_id, state, events, retry_count) VALUES ($1, 'after', 'pending', '[]'::jsonb, 0)`, caseID); err != nil {
		t.Fatalf("insert case step after: %v", err)
	}

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	taskSvc := tasks.NewTaskService(db, en, nil)
	agentExec := agents.NewAgentExecutor(agents.ExecutorConfig{DB: db, TaskCreator: taskSvc, LLMClient: agents.NewLLMClient(srv.URL, "test-model", "", 2*time.Second)})
	en.RegisterExecutor("agent", agentExec)
	en.RegisterExecutor("rule", engine.NewMockExecutor(map[string][]engine.MockExecution{"after": {{Result: &engine.StepResult{Output: json.RawMessage(`{"ok":true}`)}}}}))

	if err := en.EvaluateDAG(ctx, caseID); err != nil {
		t.Fatalf("evaluate dag: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "agent", engine.StateActive)

	if err := taskSvc.CompleteTask(ctx, tenantID, principalID, caseID, "agent", tasks.CompleteTaskRequest{Outcome: "accept", Data: map[string]any{}}); err != nil {
		t.Fatalf("complete agent review task: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "agent", engine.StateCompleted)
	waitForStepState(t, ctx, db, caseID, "after", engine.StateCompleted)
}

func TestAgentsIntegration_MetricsTokensAndConfidence(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "a006-metrics")
	caseTypeID := seedAdditionalCaseType(t, ctx, db, tenantID, principalID, "agent_metrics_case")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "test-model",
				"choices": []map[string]any{{
					"finish_reason": "stop",
					"message":       map[string]any{"content": `{"score":77,"risk_level":"medium","reasoning":"ok","flags":["f1"],"confidence":0.83}`},
				}},
				"usage": map[string]any{"prompt_tokens": 21, "completion_tokens": 13},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	promptSvc := agents.NewPromptTemplateService(db)
	if _, err := promptSvc.Create(ctx, tenantID, principalID, agents.CreatePromptTemplateRequest{
		Name:     "risk_metrics",
		Template: `Case {{.case.applicant.company_name}}`,
	}); err != nil {
		t.Fatalf("create prompt template: %v", err)
	}

	stepCfg, _ := json.Marshal(map[string]any{
		"prompt_template":      "risk_metrics_v1",
		"model":                "test-model",
		"context":              []map[string]any{{"source": "case", "fields": []string{"applicant.company_name"}}},
		"output_schema":        map[string]any{"score": map[string]any{"type": "number"}, "risk_level": map[string]any{"type": "string"}, "reasoning": map[string]any{"type": "text"}, "flags": map[string]any{"type": "array", "items": "string"}, "confidence": map[string]any{"type": "number", "min": 0, "max": 1}},
		"confidence_threshold": 0.5,
		"max_attempts":         1,
	})

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "agent", Type: "agent", Config: stepCfg}}}
	workflowID, _ := seedPublishedWorkflow(t, ctx, db, tenantID, principalID, "agent_metrics_case", ast)

	var caseID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, 'AGENT-000099', 'open', '{"applicant":{"company_name":"Acme"}}'::jsonb, $3, $4, 1)
RETURNING id
`, tenantID, caseTypeID, principalID, workflowID).Scan(&caseID); err != nil {
		t.Fatalf("insert case: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO case_steps (case_id, step_id, state, events, retry_count) VALUES ($1, 'agent', 'pending', '[]'::jsonb, 0)`, caseID); err != nil {
		t.Fatalf("insert case step: %v", err)
	}

	beforeIn := testutil.ToFloat64(observability.AgentTokensTotal.WithLabelValues(tenantID.String(), "test-model", "input"))
	beforeOut := testutil.ToFloat64(observability.AgentTokensTotal.WithLabelValues(tenantID.String(), "test-model", "output"))
	beforeConf := testutil.CollectAndCount(observability.AgentConfidenceScore)

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	agentExec := agents.NewAgentExecutor(agents.ExecutorConfig{DB: db, LLMClient: agents.NewLLMClient(srv.URL, "test-model", "", 2*time.Second)})
	en.RegisterExecutor("agent", agentExec)

	if err := en.EvaluateDAG(ctx, caseID); err != nil {
		t.Fatalf("evaluate dag: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "agent", engine.StateCompleted)

	afterIn := testutil.ToFloat64(observability.AgentTokensTotal.WithLabelValues(tenantID.String(), "test-model", "input"))
	afterOut := testutil.ToFloat64(observability.AgentTokensTotal.WithLabelValues(tenantID.String(), "test-model", "output"))
	afterConf := testutil.CollectAndCount(observability.AgentConfidenceScore)

	if afterIn <= beforeIn || afterOut <= beforeOut {
		t.Fatalf("expected token counters to increase: input %f->%f output %f->%f", beforeIn, afterIn, beforeOut, afterOut)
	}
	if afterConf <= beforeConf {
		t.Fatalf("expected confidence histogram count to increase: %d->%d", beforeConf, afterConf)
	}
}
