package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (a *AgentExecutor) invokeWithValidationRetry(ctx context.Context, model string, renderedPrompt string, cfg StepConfig) (map[string]any, Usage, int, error) {
	start := time.Now()
	messages := []Message{
		{Role: "system", Content: "You are an assistant that must return strict JSON matching the requested schema."},
		{Role: "user", Content: renderedPrompt},
	}
	responseFormat := &ResponseFormat{Type: "json_schema", JSONSchema: map[string]any{"name": "agent_output", "schema": cfg.OutputSchema}}

	var lastErr error
	var usage Usage
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		client := *a.llm
		client.model = model
		llmCtx, cancel := context.WithTimeout(ctx, a.llmTimeout)
		resp, err := client.ChatCompletion(llmCtx, messages, responseFormat)
		cancel()
		if err != nil {
			return nil, usage, 0, err
		}
		usage = resp.Usage

		parsed := map[string]any{}
		if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
			lastErr = fmt.Errorf("invalid json output: %w", err)
		} else if err := validateOutputAgainstSchema(parsed, cfg.OutputSchema); err != nil {
			lastErr = err
		} else {
			if _, ok := asFloat(parsed["confidence"]); !ok {
				lastErr = fmt.Errorf("missing confidence field")
			} else {
				return parsed, usage, int(time.Since(start).Milliseconds()), nil
			}
		}

		corrective := fmt.Sprintf("Your previous response did not match the required schema. Error: %s. Please respond again with valid JSON matching: %s", lastErr.Error(), mustJSON(cfg.OutputSchema))
		messages = []Message{{Role: "system", Content: "You must return only valid JSON."}, {Role: "user", Content: corrective}}
	}
	if lastErr == nil {
		lastErr = errors.New("validation failed")
	}
	return nil, usage, int(time.Since(start).Milliseconds()), lastErr
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
