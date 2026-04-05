package agentic

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/llm"
)

type LLMManager interface {
	Chat(ctx context.Context, tenantID uuid.UUID, req llm.ChatRequest) (llm.ChatResponse, error)
}

// AgenticRunner is the Aceryx-owned interface for the reasoning loop.
type AgenticRunner interface {
	Run(ctx context.Context, config RunConfig) (RunResult, error)
}

type RunConfig struct {
	TenantID     uuid.UUID
	CaseID       uuid.UUID
	StepID       string
	InstanceID   uuid.UUID
	Goal         string
	CaseData     json.RawMessage
	ToolManifest *ToolManifest
	Limits       ReasoningLimits
	OutputSchema json.RawMessage
	LLMAdapter   LLMManager
	TraceStore   TraceStore
	Model        string
}

type RunResult struct {
	Conclusion      json.RawMessage
	Confidence      *float64
	Status          string
	TotalIterations int
	TotalToolCalls  int
	TotalTokens     int
	DurationMS      int
	TraceID         uuid.UUID
}

type runner struct{}

func NewRunner() AgenticRunner {
	return &runner{}
}

func (r *runner) Run(ctx context.Context, config RunConfig) (RunResult, error) {
	started := time.Now()
	config.Limits.ApplyDefaults()
	ce := NewConstraintEnforcer(config.Limits)

	trace := &ReasoningTrace{
		ID:         uuid.New(),
		TenantID:   config.TenantID,
		CaseID:     config.CaseID,
		StepID:     config.StepID,
		InstanceID: config.InstanceID,
		ModelUsed:  config.Model,
		Goal:       config.Goal,
		Status:     "running",
		CreatedAt:  started.UTC(),
	}
	if err := config.TraceStore.CreateTrace(ctx, trace); err != nil {
		return RunResult{}, err
	}
	_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
		TraceID:   trace.ID,
		Iteration: 0,
		Sequence:  1,
		EventType: "goal_set",
		Content:   mustJSON(map[string]any{"goal": config.Goal}),
	})

	messages := []llm.Message{
		{Role: "user", Content: buildGoalPrompt(config.Goal, config.CaseData, config.OutputSchema)},
	}
	systemPrompt := buildSystemPrompt(config.ToolManifest, config.OutputSchema)
	toolDefs := config.ToolManifest.ToLLMToolDefs()

	iteration := 0
	toolCallCount := 0
	totalTokens := 0
	invalidConclusionAttempts := 0
	lastResponseContent := ""
	lastResponseJSON := json.RawMessage(`{}`)

	for {
		if ce.CheckTimeout() {
			return r.finish(ctx, config.TraceStore, trace, RunResult{
				Status:          "timeout",
				TraceID:         trace.ID,
				TotalIterations: iteration,
				TotalToolCalls:  toolCallCount,
				TotalTokens:     totalTokens,
				DurationMS:      int(time.Since(started).Milliseconds()),
				Conclusion:      lastResponseJSON,
			})
		}
		iteration++
		ce.IncrementIteration()
		sequence := 0
		if msg := ce.CheckIteration(); msg != "" {
			messages = append(messages, llm.Message{Role: "user", Content: msg})
		}

		messages = summariseOldToolResults(messages, 24)
		resp, err := config.LLMAdapter.Chat(ctx, config.TenantID, llm.ChatRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Model:        config.Model,
			Tools:        toolDefs,
			ToolChoice:   "auto",
			JSONMode:     false,
			MaxTokens:    max(128, config.Limits.MaxTokens-totalTokens),
			Purpose:      "agentic",
		})
		if err != nil {
			_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
				TraceID:   trace.ID,
				Iteration: iteration,
				Sequence:  sequence + 1,
				EventType: "error",
				Content:   mustJSON(map[string]any{"error": err.Error()}),
			})
			return r.finish(ctx, config.TraceStore, trace, RunResult{
				Status:          "error",
				TraceID:         trace.ID,
				TotalIterations: iteration,
				TotalToolCalls:  toolCallCount,
				TotalTokens:     totalTokens,
				DurationMS:      int(time.Since(started).Milliseconds()),
				Conclusion:      lastResponseJSON,
			})
		}
		lastResponseContent = resp.Content
		lastResponseJSON = json.RawMessage(strings.TrimSpace(resp.Content))
		totalTokens += resp.TotalTokens
		ce.SetTokensUsed(totalTokens)

		sequence++
		_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
			TraceID:    trace.ID,
			Iteration:  iteration,
			Sequence:   sequence,
			EventType:  "reasoning",
			Content:    mustJSON(map[string]any{"content": resp.Content}),
			TokenCount: resp.TotalTokens,
		})

		if warning, hardStop := ce.CheckTokenBudget(totalTokens); hardStop {
			conclusion, conf, cerr := parseConclusion(lastResponseContent, config.OutputSchema)
			if cerr != nil {
				conclusion = mustJSON(map[string]any{"raw_response": lastResponseContent})
				conf = nil
			}
			return r.finish(ctx, config.TraceStore, trace, RunResult{
				Status:          "timeout",
				TraceID:         trace.ID,
				Conclusion:      conclusion,
				Confidence:      conf,
				TotalIterations: iteration,
				TotalToolCalls:  toolCallCount,
				TotalTokens:     totalTokens,
				DurationMS:      int(time.Since(started).Milliseconds()),
			})
		} else if warning != "" {
			messages = append(messages, llm.Message{Role: "user", Content: warning})
		}

		if resp.FinishReason == "tool_calls" && len(resp.ToolCalls) > 0 {
			messages = append(messages, llm.Message{Role: "assistant", ToolCalls: resp.ToolCalls})
			for _, tc := range resp.ToolCalls {
				sequence++
				if msg := ce.CheckToolCalls(); msg != "" {
					messages = append(messages, llm.Message{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    msg,
					})
					_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
						TraceID:    trace.ID,
						Iteration:  iteration,
						Sequence:   sequence,
						EventType:  "error",
						Content:    mustJSON(map[string]any{"tool": tc.Name, "error": msg}),
						TokenCount: 0,
					})
					continue
				}
				toolCallCount++
				ce.IncrementToolCalls()

				tool, err := ValidateToolCall(tc.Name, config.ToolManifest)
				if err != nil {
					messages = append(messages, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: err.Error()})
					_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
						TraceID:   trace.ID,
						Iteration: iteration,
						Sequence:  sequence,
						EventType: "error",
						Content:   mustJSON(map[string]any{"tool": tc.Name, "error": err.Error()}),
					})
					continue
				}
				if err := ValidateToolArgs(tc.Arguments, tool.Parameters); err != nil {
					messages = append(messages, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: err.Error()})
					_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
						TraceID:   trace.ID,
						Iteration: iteration,
						Sequence:  sequence,
						EventType: "error",
						Content:   mustJSON(map[string]any{"tool": tc.Name, "error": err.Error()}),
					})
					continue
				}

				_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
					TraceID:    trace.ID,
					Iteration:  iteration,
					Sequence:   sequence,
					EventType:  "tool_call",
					Content:    mustJSON(map[string]any{"id": tc.ID, "name": tc.Name, "arguments": tc.Arguments}),
					ToolID:     tool.ID,
					ToolSource: string(tool.Source),
					ToolSafety: tool.ToolSafety,
					SideEffect: tool.ToolSafety == "side_effect",
				})

				callStarted := time.Now()
				result, err := tool.Invoker.Invoke(ctx, []byte(tc.Arguments))
				duration := int(time.Since(callStarted).Milliseconds())
				if err != nil {
					messages = append(messages, llm.Message{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    "Tool error: " + err.Error(),
					})
					_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
						TraceID:    trace.ID,
						Iteration:  iteration,
						Sequence:   sequence + 1,
						EventType:  "tool_result",
						Content:    mustJSON(map[string]any{"error": err.Error()}),
						ToolID:     tool.ID,
						ToolSource: string(tool.Source),
						ToolSafety: tool.ToolSafety,
						SideEffect: tool.ToolSafety == "side_effect",
						DurationMS: duration,
					})
					continue
				}
				messages = append(messages, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    string(result),
				})
				_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
					TraceID:    trace.ID,
					Iteration:  iteration,
					Sequence:   sequence + 1,
					EventType:  "tool_result",
					Content:    result,
					ToolID:     tool.ID,
					ToolSource: string(tool.Source),
					ToolSafety: tool.ToolSafety,
					SideEffect: tool.ToolSafety == "side_effect",
					DurationMS: duration,
				})
			}
			continue
		}

		conclusion, confidence, err := parseConclusion(resp.Content, config.OutputSchema)
		if err != nil {
			invalidConclusionAttempts++
			_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
				TraceID:   trace.ID,
				Iteration: iteration,
				Sequence:  sequence + 1,
				EventType: "error",
				Content:   mustJSON(map[string]any{"error": err.Error(), "response": resp.Content}),
			})
			if invalidConclusionAttempts >= 3 {
				return r.finish(ctx, config.TraceStore, trace, RunResult{
					Status:          "error",
					TraceID:         trace.ID,
					TotalIterations: iteration,
					TotalToolCalls:  toolCallCount,
					TotalTokens:     totalTokens,
					DurationMS:      int(time.Since(started).Milliseconds()),
					Conclusion:      mustJSON(map[string]any{"raw_response": resp.Content}),
				})
			}
			messages = append(messages,
				llm.Message{Role: "assistant", Content: resp.Content},
				llm.Message{Role: "user", Content: "Your previous conclusion did not match the required JSON output schema. Return valid JSON only."},
			)
			continue
		}
		_ = config.TraceStore.AppendEvent(ctx, &ReasoningEvent{
			TraceID:   trace.ID,
			Iteration: iteration,
			Sequence:  sequence + 2,
			EventType: "conclusion",
			Content:   conclusion,
		})
		return r.finish(ctx, config.TraceStore, trace, RunResult{
			Status:          "concluded",
			TraceID:         trace.ID,
			Conclusion:      conclusion,
			Confidence:      confidence,
			TotalIterations: iteration,
			TotalToolCalls:  toolCallCount,
			TotalTokens:     totalTokens,
			DurationMS:      int(time.Since(started).Milliseconds()),
		})
	}
}

func (r *runner) finish(ctx context.Context, store TraceStore, trace *ReasoningTrace, result RunResult) (RunResult, error) {
	now := time.Now().UTC()
	trace.Status = result.Status
	trace.Conclusion = result.Conclusion
	trace.TotalIterations = result.TotalIterations
	trace.TotalToolCalls = result.TotalToolCalls
	trace.TotalTokens = result.TotalTokens
	trace.TotalDurationMS = result.DurationMS
	trace.CompletedAt = &now
	if err := store.UpdateTrace(ctx, trace); err != nil {
		return RunResult{}, err
	}
	result.TraceID = trace.ID
	return result, nil
}

func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}
