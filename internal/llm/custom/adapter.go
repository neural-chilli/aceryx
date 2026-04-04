package custom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/llm"
)

type Config struct {
	APIKey          string
	BaseURL         string
	DefaultModel    string
	HTTPClient      *http.Client
	Models          []llm.ModelInfo
	Azure           bool
	AzureAPIVersion string
}

type Adapter struct {
	baseURL         string
	defaultModel    string
	apiKey          string
	httpClient      *http.Client
	models          []llm.ModelInfo
	azure           bool
	azureAPIVersion string
}

func New(cfg Config) *Adapter {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080/v1"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	return &Adapter{
		baseURL:         baseURL,
		defaultModel:    strings.TrimSpace(cfg.DefaultModel),
		apiKey:          strings.TrimSpace(cfg.APIKey),
		httpClient:      httpClient,
		models:          append([]llm.ModelInfo(nil), cfg.Models...),
		azure:           cfg.Azure,
		azureAPIVersion: strings.TrimSpace(cfg.AzureAPIVersion),
	}
}

func (a *Adapter) Provider() string { return "custom" }

func (a *Adapter) SupportsVision() bool { return true }

func (a *Adapter) SupportsJSON() bool { return false }

func (a *Adapter) Models() []llm.ModelInfo { return append([]llm.ModelInfo(nil), a.models...) }

func (a *Adapter) Close() error { return nil }

func (a *Adapter) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = a.defaultModel
	}
	payload := map[string]any{
		"model":    model,
		"messages": toMessages(req),
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}
	if req.JSONMode {
		// Generic compatibility mode: prompt augmentation.
		payload["messages"] = append([]map[string]any{{
			"role":    "system",
			"content": "Return only valid JSON. No commentary, no markdown fences.",
		}}, toMessages(req)...)
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toTools(req.Tools)
	}
	if tc := strings.TrimSpace(req.ToolChoice); tc != "" {
		payload["tool_choice"] = tc
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
		return llm.ChatResponse{}, fmt.Errorf("decode custom chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return llm.ChatResponse{}, fmt.Errorf("custom chat response has no choices")
	}
	out := llm.ChatResponse{
		Content:      decoded.Choices[0].Message.Content,
		InputTokens:  decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens,
		TotalTokens:  decoded.Usage.TotalTokens,
		Model:        decoded.Model,
		FinishReason: normalizeFinish(decoded.Choices[0].FinishReason),
	}
	for _, tc := range decoded.Choices[0].Message.ToolCalls {
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
	model = strings.TrimSpace(model)
	if model == "" {
		model = a.defaultModel
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
		return nil, fmt.Errorf("decode custom embeddings response: %w", err)
	}
	out := make([][]float32, 0, len(decoded.Data))
	for _, row := range decoded.Data {
		out = append(out, row.Embedding)
	}
	return out, nil
}

func (a *Adapter) doJSON(ctx context.Context, method, path string, payload any) (int, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal custom request: %w", err)
	}
	endpoint := a.baseURL + path
	if a.azure && a.azureAPIVersion != "" {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return 0, nil, fmt.Errorf("parse custom endpoint: %w", err)
		}
		q := parsed.Query()
		q.Set("api-version", a.azureAPIVersion)
		parsed.RawQuery = q.Encode()
		endpoint = parsed.String()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, fmt.Errorf("build custom request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		if a.azure {
			req.Header.Set("api-key", a.apiKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+a.apiKey)
		}
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("custom request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read custom response: %w", err)
	}
	return resp.StatusCode, body, nil
}

func toMessages(req llm.ChatRequest) []map[string]any {
	msgs := make([]map[string]any, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": req.SystemPrompt})
	}
	imagesAttached := false
	for i, msg := range req.Messages {
		entry := map[string]any{"role": msg.Role}
		attachImages := !imagesAttached && msg.Role == "user" && len(req.Images) > 0 && i == len(req.Messages)-1
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
			calls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]string{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			entry["tool_calls"] = calls
		}
		msgs = append(msgs, entry)
	}
	return msgs
}

func toTools(tools []llm.ToolDef) []map[string]any {
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
