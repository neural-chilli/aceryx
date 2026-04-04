package ollama

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

type Config struct {
	BaseURL      string
	DefaultModel string
	HTTPClient   *http.Client
	Models       []llm.ModelInfo
}

type Adapter struct {
	chatURL       string
	embeddingsURL string
	defaultModel  string
	httpClient    *http.Client
	models        []llm.ModelInfo
}

func New(cfg Config) *Adapter {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	models := cfg.Models
	if len(models) == 0 {
		models = []llm.ModelInfo{
			{ID: "llama3.1:8b", Name: "Llama 3.1 8B", ContextWindow: 8192, SupportsJSON: true},
		}
	}
	return &Adapter{
		chatURL:       baseURL + "/v1/chat/completions",
		embeddingsURL: baseURL + "/api/embeddings",
		defaultModel:  cfg.DefaultModel,
		httpClient:    httpClient,
		models:        models,
	}
}

func (a *Adapter) Provider() string { return "ollama" }

func (a *Adapter) SupportsVision() bool {
	for _, m := range a.models {
		if m.SupportsVision {
			return true
		}
	}
	return false
}

func (a *Adapter) SupportsJSON() bool { return true }

func (a *Adapter) Models() []llm.ModelInfo { return append([]llm.ModelInfo(nil), a.models...) }

func (a *Adapter) Close() error { return nil }

func (a *Adapter) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(a.defaultModel)
	}
	if model == "" && len(a.models) > 0 {
		model = a.models[0].ID
	}

	// Ollama supports OpenAI-compatible chat completions, with optional format=json.
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
		payload["format"] = "json"
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toOpenAITools(req.Tools)
	}
	if tc := strings.TrimSpace(req.ToolChoice); tc != "" {
		payload["tool_choice"] = tc
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("marshal ollama chat request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.chatURL, bytes.NewReader(raw))
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("build ollama chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("ollama chat request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("read ollama chat response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return llm.ChatResponse{}, statusErr(resp.StatusCode, body)
	}

	var decoded struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
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
		return llm.ChatResponse{}, fmt.Errorf("decode ollama chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return llm.ChatResponse{}, fmt.Errorf("ollama response has no choices")
	}
	choice := decoded.Choices[0]
	out := llm.ChatResponse{
		Content:      choice.Message.Content,
		InputTokens:  decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens,
		TotalTokens:  decoded.Usage.TotalTokens,
		Model:        decoded.Model,
		FinishReason: normalizeFinish(choice.FinishReason),
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
		model = strings.TrimSpace(a.defaultModel)
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		payload := map[string]any{"model": model, "prompt": text}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal ollama embeddings request: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.embeddingsURL, bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("build ollama embeddings request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollama embeddings request failed: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read ollama embeddings response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, statusErr(resp.StatusCode, body)
		}
		var decoded struct {
			Embedding []float32 `json:"embedding"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("decode ollama embeddings response: %w", err)
		}
		out = append(out, decoded.Embedding)
	}
	return out, nil
}

func toOpenAITools(tools []llm.ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		if strings.TrimSpace(t.Name) == "" {
			continue
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}
	return out
}

func toOpenAIMessages(req llm.ChatRequest) []map[string]any {
	messages := make([]llm.Message, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		messages = append(messages, llm.Message{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, req.Messages...)

	out := make([]map[string]any, 0, len(messages))
	imagesAttached := false
	for i, msg := range messages {
		entry := map[string]any{"role": msg.Role}
		attachImages := !imagesAttached && msg.Role == "user" && len(req.Images) > 0 && i == len(messages)-1
		if attachImages {
			content := make([]map[string]any, 0, len(req.Images)+1)
			if strings.TrimSpace(msg.Content) != "" {
				content = append(content, map[string]any{"type": "text", "text": msg.Content})
			}
			for _, image := range req.Images {
				url := strings.TrimSpace(image.URL)
				if url == "" && strings.TrimSpace(image.Base64) != "" {
					mime := strings.TrimSpace(image.MimeType)
					if mime == "" {
						mime = "image/png"
					}
					url = "data:" + mime + ";base64," + strings.TrimSpace(image.Base64)
				}
				if url == "" {
					continue
				}
				content = append(content, map[string]any{
					"type": "image_url",
					"image_url": map[string]string{
						"url": url,
					},
				})
			}
			entry["content"] = content
			imagesAttached = true
		} else {
			entry["content"] = msg.Content
		}
		if msg.ToolCallID != "" {
			entry["tool_call_id"] = msg.ToolCallID
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
			entry["tool_calls"] = toolCalls
		}
		out = append(out, entry)
	}
	return out
}

func normalizeFinish(reason string) string {
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
	msg := strings.TrimSpace(string(body))
	hs := &llm.HTTPStatusError{StatusCode: status, Body: msg}
	if status == http.StatusTooManyRequests {
		return fmt.Errorf("%w: %v", llm.ErrRateLimited, hs)
	}
	if status >= 500 {
		return fmt.Errorf("%w: %v", llm.ErrProviderUnavailable, hs)
	}
	return hs
}

func ensureJSON(content string) error {
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
