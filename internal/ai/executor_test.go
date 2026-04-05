package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/llm"
)

type mockLLM struct {
	responses []llm.ChatResponse
	errs      []error
	calls     []llm.ChatRequest
	hints     []string
}

func (m *mockLLM) ResolveModel(_ uuid.UUID, hint string) string {
	m.hints = append(m.hints, hint)
	switch hint {
	case "vision":
		return "vision-model"
	case "small":
		return "small-model"
	case "large":
		return "large-model"
	default:
		return "medium-model"
	}
}

func (m *mockLLM) Chat(_ context.Context, _ uuid.UUID, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.calls = append(m.calls, req)
	i := len(m.calls) - 1
	if i < len(m.errs) && m.errs[i] != nil {
		return llm.ChatResponse{}, m.errs[i]
	}
	if i >= len(m.responses) {
		return llm.ChatResponse{}, errors.New("missing mocked response")
	}
	return m.responses[i], nil
}

type mockCaseStore struct {
	record CaseRecord
	err    error
}

func (m *mockCaseStore) GetCaseContext(_ context.Context, _ uuid.UUID, _ uuid.UUID) (CaseRecord, error) {
	if m.err != nil {
		return CaseRecord{}, m.err
	}
	return m.record, nil
}

type mockTaskStore struct {
	requests []ReviewTaskRequest
	err      error
}

func (m *mockTaskStore) CreateReviewTask(_ context.Context, req ReviewTaskRequest) error {
	if m.err != nil {
		return m.err
	}
	m.requests = append(m.requests, req)
	return nil
}

func TestExecutorFullPathAccepted(t *testing.T) {
	tenantID := uuid.New()
	caseID := uuid.New()
	reg := NewComponentRegistry(nil)
	reg.global["sentiment"] = &AIComponentDef{
		ID:             "sentiment",
		DisplayLabel:   "Sentiment",
		Category:       "AI: Text Analysis",
		Tier:           TierCommercial,
		InputSchema:    json.RawMessage(`{"type":"object"}`),
		OutputSchema:   json.RawMessage(`{"type":"object","properties":{"sentiment":{"type":"string"},"confidence":{"type":"number"}},"required":["sentiment","confidence"]}`),
		SystemPrompt:   "Return JSON",
		UserPromptTmpl: "{{.Input.text}}",
		ModelHints:     ModelHints{PreferredSize: "small", MaxTokens: 128},
		Confidence:     &ConfidenceConfig{FieldPath: "confidence", AutoAcceptAbove: 0.9, EscalateBelow: 0.6},
	}
	llmMock := &mockLLM{responses: []llm.ChatResponse{{
		Content:      `{"sentiment":"positive","confidence":0.95}`,
		InputTokens:  11,
		OutputTokens: 7,
		TotalTokens:  18,
		Model:        "small-model",
	}}}
	caseStore := &mockCaseStore{record: CaseRecord{ID: caseID, TenantID: tenantID, CaseType: "support", Data: map[string]any{"message": "great service"}}}
	taskStore := &mockTaskStore{}
	ex := NewComponentExecutor(llmMock, caseStore, taskStore, reg)

	res, err := ex.Execute(context.Background(), ComponentExecRequest{
		TenantID:    tenantID,
		CaseID:      caseID,
		StepID:      "s1",
		ComponentID: "sentiment",
		InputPaths:  map[string]string{"text": "message"},
		OutputPath:  "ai.sentiment",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Status != "accepted" {
		t.Fatalf("expected accepted, got %s", res.Status)
	}
	if len(taskStore.requests) != 0 {
		t.Fatalf("expected no review task")
	}
	if len(res.MergePatch) == 0 {
		t.Fatalf("expected merge patch")
	}
	if len(llmMock.calls) != 1 || !llmMock.calls[0].JSONMode {
		t.Fatalf("expected one JSON-mode llm call")
	}
	if llmMock.hints[0] != "small" {
		t.Fatalf("expected small model hint, got %q", llmMock.hints[0])
	}
}

func TestExecutorValidationRetryThenSuccess(t *testing.T) {
	tenantID := uuid.New()
	caseID := uuid.New()
	reg := NewComponentRegistry(nil)
	reg.global["k"] = &AIComponentDef{
		ID:             "k",
		DisplayLabel:   "K",
		Category:       "AI: Text Analysis",
		Tier:           TierCommercial,
		InputSchema:    json.RawMessage(`{"type":"object"}`),
		OutputSchema:   json.RawMessage(`{"type":"object","properties":{"result":{"type":"string"}},"required":["result"]}`),
		SystemPrompt:   "sys",
		UserPromptTmpl: "{{.Input.text}}",
		ModelHints:     ModelHints{PreferredSize: "small", MaxTokens: 100},
	}
	llmMock := &mockLLM{responses: []llm.ChatResponse{
		{Content: `{"wrong":"x"}`},
		{Content: `{"result":"ok"}`},
	}}
	ex := NewComponentExecutor(llmMock, &mockCaseStore{record: CaseRecord{ID: caseID, TenantID: tenantID, CaseType: "x", Data: map[string]any{"text": "hello"}}}, &mockTaskStore{}, reg)
	res, err := ex.Execute(context.Background(), ComponentExecRequest{
		TenantID: tenantID, CaseID: caseID, StepID: "s", ComponentID: "k", InputPaths: map[string]string{"text": "text"}, OutputPath: "out",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Status != "accepted" {
		t.Fatalf("expected accepted")
	}
	if len(llmMock.calls) != 2 {
		t.Fatalf("expected two llm calls, got %d", len(llmMock.calls))
	}
}

func TestExecutorValidationRetriesExhausted(t *testing.T) {
	tenantID := uuid.New()
	caseID := uuid.New()
	reg := NewComponentRegistry(nil)
	reg.global["k"] = &AIComponentDef{
		ID: "k", DisplayLabel: "K", Category: "AI", Tier: TierCommercial,
		InputSchema:  json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"result":{"type":"string"}},"required":["result"]}`),
		SystemPrompt: "sys", UserPromptTmpl: "{{.Input.text}}", ModelHints: ModelHints{PreferredSize: "small", MaxTokens: 100},
	}
	llmMock := &mockLLM{responses: []llm.ChatResponse{{Content: `{"wrong":1}`}, {Content: `{"wrong":2}`}, {Content: `{"wrong":3}`}}}
	ex := NewComponentExecutor(llmMock, &mockCaseStore{record: CaseRecord{ID: caseID, TenantID: tenantID, CaseType: "x", Data: map[string]any{"text": "hello"}}}, &mockTaskStore{}, reg)
	_, err := ex.Execute(context.Background(), ComponentExecRequest{TenantID: tenantID, CaseID: caseID, StepID: "s", ComponentID: "k", InputPaths: map[string]string{"text": "text"}, OutputPath: "out"})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestExecutorConfidenceEscalationAndWarningAndNoConfidence(t *testing.T) {
	tenantID := uuid.New()
	caseID := uuid.New()
	baseDef := &AIComponentDef{ID: "c", DisplayLabel: "C", Category: "AI", Tier: TierCommercial, InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object","properties":{"confidence":{"type":"number"}},"required":["confidence"]}`), SystemPrompt: "sys", UserPromptTmpl: "{{.Input.t}}", ModelHints: ModelHints{PreferredSize: "small", MaxTokens: 10}}

	reg := NewComponentRegistry(nil)
	reg.global["c"] = cloneComponent(baseDef)
	reg.global["c"].Confidence = &ConfidenceConfig{FieldPath: "confidence", AutoAcceptAbove: 0.9, EscalateBelow: 0.6}
	tasks1 := &mockTaskStore{}
	ex1 := NewComponentExecutor(&mockLLM{responses: []llm.ChatResponse{{Content: `{"confidence":0.5}`}}}, &mockCaseStore{record: CaseRecord{ID: caseID, TenantID: tenantID, CaseType: "t", Data: map[string]any{"t": "x"}}}, tasks1, reg)
	res1, err := ex1.Execute(context.Background(), ComponentExecRequest{TenantID: tenantID, CaseID: caseID, StepID: "s", ComponentID: "c", InputPaths: map[string]string{"t": "t"}, OutputPath: "o"})
	if err != nil {
		t.Fatalf("execute escalation: %v", err)
	}
	if res1.Status != "escalated" || len(tasks1.requests) != 1 || len(res1.MergePatch) != 0 {
		t.Fatalf("expected escalation with task and no merge patch")
	}

	tasks2 := &mockTaskStore{}
	ex2 := NewComponentExecutor(&mockLLM{responses: []llm.ChatResponse{{Content: `{"confidence":0.75}`}}}, &mockCaseStore{record: CaseRecord{ID: caseID, TenantID: tenantID, CaseType: "t", Data: map[string]any{"t": "x"}}}, tasks2, reg)
	res2, err := ex2.Execute(context.Background(), ComponentExecRequest{TenantID: tenantID, CaseID: caseID, StepID: "s", ComponentID: "c", InputPaths: map[string]string{"t": "t"}, OutputPath: "o"})
	if err != nil {
		t.Fatalf("execute warning: %v", err)
	}
	if res2.Status != "warning" || !res2.ConfidenceWarning || len(res2.MergePatch) == 0 {
		t.Fatalf("expected warning with merge patch")
	}

	reg.global["c2"] = cloneComponent(baseDef)
	ex3 := NewComponentExecutor(&mockLLM{responses: []llm.ChatResponse{{Content: `{"confidence":0.1}`}}}, &mockCaseStore{record: CaseRecord{ID: caseID, TenantID: tenantID, CaseType: "t", Data: map[string]any{"t": "x"}}}, &mockTaskStore{}, reg)
	res3, err := ex3.Execute(context.Background(), ComponentExecRequest{TenantID: tenantID, CaseID: caseID, StepID: "s", ComponentID: "c2", InputPaths: map[string]string{"t": "t"}, OutputPath: "o"})
	if err != nil {
		t.Fatalf("execute no confidence: %v", err)
	}
	if res3.Status != "accepted" {
		t.Fatalf("expected accepted when no confidence config")
	}
}

func TestExecutorModelHintsVision(t *testing.T) {
	tenantID := uuid.New()
	caseID := uuid.New()
	reg := NewComponentRegistry(nil)
	reg.global["v"] = &AIComponentDef{
		ID: "v", DisplayLabel: "V", Category: "AI", Tier: TierCommercial,
		InputSchema:  json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"result":{"type":"string"}},"required":["result"]}`),
		SystemPrompt: "sys", UserPromptTmpl: "{{.Input.text}}", ModelHints: ModelHints{RequiresVision: true, MaxTokens: 40},
	}
	llmMock := &mockLLM{responses: []llm.ChatResponse{{Content: `{"result":"ok"}`}}}
	ex := NewComponentExecutor(llmMock, &mockCaseStore{record: CaseRecord{ID: caseID, TenantID: tenantID, CaseType: "t", Data: map[string]any{"text": "x"}}}, &mockTaskStore{}, reg)
	_, err := ex.Execute(context.Background(), ComponentExecRequest{TenantID: tenantID, CaseID: caseID, StepID: "s", ComponentID: "v", InputPaths: map[string]string{"text": "text"}, OutputPath: "out"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(llmMock.hints) != 1 || llmMock.hints[0] != "vision" {
		t.Fatalf("expected vision hint, got %#v", llmMock.hints)
	}
}
