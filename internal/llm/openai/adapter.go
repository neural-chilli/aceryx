package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/llm"
)

const defaultBaseURL = "https://api.openai.com/v1"

type Config struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	HTTPClient   *http.Client
	Models       []llm.ModelInfo
}

type Adapter struct {
	baseURL      string
	defaultModel string
	apiKey       string
	httpClient   *http.Client
	models       []llm.ModelInfo
}

func New(cfg Config) *Adapter {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	models := cfg.Models
	if len(models) == 0 {
		models = []llm.ModelInfo{
			{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000, SupportsVision: true, SupportsJSON: true},
			{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ContextWindow: 128000, SupportsVision: true, SupportsJSON: true},
		}
	}
	return &Adapter{
		baseURL:      baseURL,
		defaultModel: strings.TrimSpace(cfg.DefaultModel),
		apiKey:       strings.TrimSpace(cfg.APIKey),
		httpClient:   httpClient,
		models:       models,
	}
}

func (a *Adapter) Provider() string { return "openai" }

func (a *Adapter) SupportsVision() bool { return true }

func (a *Adapter) SupportsJSON() bool { return true }

func (a *Adapter) Models() []llm.ModelInfo { return append([]llm.ModelInfo(nil), a.models...) }

func (a *Adapter) Close() error { return nil }

func (a *Adapter) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = a.defaultModel
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	payload := map[string]any{
		"model":    model,
		"messages": toOpenAIMessages(req),
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}
	if req.JSONMode {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toOpenAITools(req.Tools)
	}
	if toolChoice := strings.TrimSpace(req.ToolChoice); toolChoice != "" {
		switch toolChoice {
		case "auto", "required", "none":
			payload["tool_choice"] = toolChoice
		default:
			payload["tool_choice"] = map[string]any{
				"type": "function",
				"function": map[string]string{
					"name": toolChoice,
				},
			}
		}
	}

	status, body, err := a.doJSON(ctx, http.MethodPost, "/chat/completions", payload)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	if status != http.StatusOK {
		return llm.ChatResponse{}, statusErr(status, body)
	}

	var decoded struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("decode openai chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return llm.ChatResponse{}, fmt.Errorf("openai response has no choices")
	}
	choice := decoded.Choices[0]
	out := llm.ChatResponse{
		Content:      choice.Message.Content,
		InputTokens:  decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens,
		TotalTokens:  decoded.Usage.TotalTokens,
		Model:        decoded.Model,
		FinishReason: normalizeFinishReason(choice.FinishReason),
	}
	for _, tc := range choice.Message.ToolCalls {
		if tc.Function.Name == "" {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	if req.JSONMode {
		if err := ensureJSON(out.Content); err != nil {
			return llm.ChatResponse{}, err
		}
	}
	return out, nil
}

func (a *Adapter) Embed(ctx context.Context, texts []string, model string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("embedding texts are required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = a.defaultModel
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	payload := map[string]any{
		"model": model,
		"input": texts,
	}
	status, body, err := a.doJSON(ctx, http.MethodPost, "/embeddings", payload)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, statusErr(status, body)
	}
	var decoded struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode openai embeddings response: %w", err)
	}
	out := make([][]float32, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		out = append(out, item.Embedding)
	}
	return out, nil
}

func (a *Adapter) doJSON(ctx context.Context, method, path string, payload any) (int, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal openai request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, fmt.Errorf("build openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read openai response: %w", err)
	}
	return resp.StatusCode, body, nil
}

func toOpenAITools(tools []llm.ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, item := range tools {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        item.Name,
				"description": item.Description,
				"parameters":  item.Parameters,
			},
		})
	}
	return out
}

func toOpenAIMessages(req llm.ChatRequest) []map[string]any {
	msgs := make([]llm.Message, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: req.SystemPrompt})
	}
	msgs = append(msgs, req.Messages...)

	out := make([]map[string]any, 0, len(msgs))
	imagesAttached := false
	for i, msg := range msgs {
		payload := map[string]any{"role": msg.Role}
		shouldAttachImages := !imagesAttached && len(req.Images) > 0 && msg.Role == "user" && i == len(msgs)-1
		if shouldAttachImages {
			parts := make([]map[string]any, 0, 1+len(req.Images))
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, map[string]any{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, img := range req.Images {
				url := strings.TrimSpace(img.URL)
				if url == "" && strings.TrimSpace(img.Base64) != "" {
					mime := strings.TrimSpace(img.MimeType)
					if mime == "" {
						mime = "image/png"
					}
					url = "data:" + mime + ";base64," + strings.TrimSpace(img.Base64)
				}
				if url == "" {
					continue
				}
				parts = append(parts, map[string]any{
					"type": "image_url",
					"image_url": map[string]string{
						"url": url,
					},
				})
			}
			payload["content"] = parts
			imagesAttached = true
		} else {
			payload["content"] = msg.Content
		}

		if msg.Role == "tool" && strings.TrimSpace(msg.ToolCallID) != "" {
			payload["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				toolCalls = append(toolCalls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]string{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			payload["tool_calls"] = toolCalls
		}
		out = append(out, payload)
	}
	return out
}

func normalizeFinishReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "tool_calls":
		return "tool_calls"
	case "length":
		return "length"
	default:
		return "stop"
	}
}

func statusErr(status int, body []byte) error {
	message := strings.TrimSpace(string(body))
	hs := &llm.HTTPStatusError{StatusCode: status, Body: message}
	if status == http.StatusTooManyRequests {
		return fmt.Errorf("%w: %v", llm.ErrRateLimited, hs)
	}
	if status >= 500 {
		return fmt.Errorf("%w: %v", llm.ErrProviderUnavailable, hs)
	}
	return hs
}

func ensureJSON(content string) error {
	if err := llmEnsureJSONObject(content); err != nil {
		return err
	}
	return nil
}

func llmEnsureJSONObject(content string) error {
	// kept local to avoid exporting utility-only helpers in parent package.
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("empty json response")
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return fmt.Errorf("invalid json response: %w", err)
	}
	return nil
}
