package agentic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/llm"
)

type mockLLM struct {
	responses []llm.ChatResponse
	i         int
}

func (m *mockLLM) Chat(_ context.Context, _ uuid.UUID, _ llm.ChatRequest) (llm.ChatResponse, error) {
	if m.i >= len(m.responses) {
		return llm.ChatResponse{Content: `{"decision":"approve","confidence":0.9}`, FinishReason: "stop", TotalTokens: 10}, nil
	}
	r := m.responses[m.i]
	m.i++
	return r, nil
}

type memoryTraceStore struct {
	trace  *ReasoningTrace
	events []*ReasoningEvent
}

func (m *memoryTraceStore) CreateTrace(_ context.Context, trace *ReasoningTrace) error {
	m.trace = trace
	return nil
}
func (m *memoryTraceStore) UpdateTrace(_ context.Context, trace *ReasoningTrace) error {
	m.trace = trace
	return nil
}
func (m *memoryTraceStore) GetTrace(_ context.Context, _ uuid.UUID) (*ReasoningTrace, error) {
	return m.trace, nil
}
func (m *memoryTraceStore) GetTraceByCase(_ context.Context, _ uuid.UUID, _ string) (*ReasoningTrace, error) {
	return m.trace, nil
}
func (m *memoryTraceStore) AppendEvent(_ context.Context, event *ReasoningEvent) error {
	m.events = append(m.events, event)
	return nil
}
func (m *memoryTraceStore) GetEvents(_ context.Context, _ uuid.UUID) ([]*ReasoningEvent, error) {
	return m.events, nil
}

type staticInvoker struct{ payload json.RawMessage }

func (s staticInvoker) Invoke(context.Context, json.RawMessage) (json.RawMessage, error) {
	return s.payload, nil
}

func TestRunner_Concludes(t *testing.T) {
	llmMock := &mockLLM{responses: []llm.ChatResponse{
		{
			Content:      "need tools",
			FinishReason: "tool_calls",
			ToolCalls:    []llm.ToolCall{{ID: "1", Name: "lookup", Arguments: `{"q":"x"}`}},
			TotalTokens:  10,
		},
		{
			Content:      `{"decision":"approve","confidence":0.91,"reasoning":["ok"]}`,
			FinishReason: "stop",
			TotalTokens:  10,
		},
	}}
	store := &memoryTraceStore{}
	manifest := NewToolManifest([]ResolvedTool{{
		ID:         "lookup1",
		Name:       "lookup",
		Parameters: []byte(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`),
		ToolSafety: "read_only",
		Invoker:    staticInvoker{payload: []byte(`{"v":1}`)},
	}})

	result, err := NewRunner().Run(context.Background(), RunConfig{
		TenantID:     uuid.New(),
		CaseID:       uuid.New(),
		StepID:       "s1",
		InstanceID:   uuid.New(),
		Goal:         "Assess",
		CaseData:     []byte(`{"a":1}`),
		ToolManifest: manifest,
		Limits:       ReasoningLimits{MaxIterations: 5, MaxToolCalls: 5, MaxTokens: 1000},
		OutputSchema: []byte(`{"type":"object","properties":{"decision":{"type":"string"},"confidence":{"type":"number"}},"required":["decision"]}`),
		LLMAdapter:   llmMock,
		TraceStore:   store,
		Model:        "x",
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Status != "concluded" {
		t.Fatalf("expected concluded, got %s", result.Status)
	}
	if len(store.events) == 0 {
		t.Fatalf("expected events")
	}
}
