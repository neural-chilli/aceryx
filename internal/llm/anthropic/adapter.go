package anthropic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/llm"
)

const defaultBaseURL = "https://api.anthropic.com/v1/messages"

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
	baseURL := strings.TrimSpace(cfg.BaseURL)
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
			{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ContextWindow: 200000, SupportsVision: true, SupportsJSON: false},
		}
	}
	return &Adapter{
		baseURL:      strings.TrimRight(baseURL, "/"),
		defaultModel: strings.TrimSpace(cfg.DefaultModel),
		apiKey:       strings.TrimSpace(cfg.APIKey),
		httpClient:   httpClient,
		models:       models,
	}
}

func (a *Adapter) Provider() string { return "anthropic" }

func (a *Adapter) SupportsVision() bool { return true }

func (a *Adapter) SupportsJSON() bool { return false }

func (a *Adapter) Models() []llm.ModelInfo { return append([]llm.ModelInfo(nil), a.models...) }

func (a *Adapter) Close() error { return nil }

func (a *Adapter) Embed(context.Context, []string, string) ([][]float32, error) {
	return nil, llm.ErrNotSupported
}

func (a *Adapter) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = a.defaultModel
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	systemPrompt := strings.TrimSpace(req.SystemPrompt)
	if req.JSONMode {
		if systemPrompt != "" {
			systemPrompt += "\n\n"
		}
		systemPrompt += "Return only valid JSON. No commentary, no markdown fences."
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": maxTokens(req.MaxTokens),
		"messages":   toAnthropicMessages(req),
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toAnthropicTools(req.Tools)
	}
	if tc := strings.TrimSpace(req.ToolChoice); tc != "" {
		switch tc {
		case "auto":
			payload["tool_choice"] = map[string]string{"type": "auto"}
		case "required":
			payload["tool_choice"] = map[string]string{"type": "any"}
		case "none":
			payload["tool_choice"] = map[string]string{"type": "none"}
		default:
			payload["tool_choice"] = map[string]string{"type": "tool", "name": tc}
		}
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("marshal anthropic request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, bytes.NewReader(raw))
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("build anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if a.apiKey != "" {
		httpReq.Header.Set("x-api-key", a.apiKey)
	}
	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("read anthropic response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return llm.ChatResponse{}, statusErr(resp.StatusCode, body)
	}

	var decoded struct {
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			ID    string `json:"id"`
			Name  string `json:"name"`
			Input any    `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("decode anthropic response: %w", err)
	}
	out := llm.ChatResponse{
		Model:        decoded.Model,
		InputTokens:  decoded.Usage.InputTokens,
		OutputTokens: decoded.Usage.OutputTokens,
		TotalTokens:  decoded.Usage.InputTokens + decoded.Usage.OutputTokens,
		FinishReason: normalizeFinish(decoded.StopReason),
	}
	var parts []string
	for _, block := range decoded.Content {
		switch block.Type {
		case "text":
			parts = append(parts, block.Text)
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(args),
			})
		}
	}
	out.Content = strings.TrimSpace(strings.Join(parts, "\n"))
	if req.JSONMode {
		if err := ensureJSON(out.Content); err != nil {
			return llm.ChatResponse{}, err
		}
	}
	return out, nil
}

func toAnthropicMessages(req llm.ChatRequest) []map[string]any {
	out := make([]map[string]any, 0, len(req.Messages))
	imagesAttached := false
	for i, msg := range req.Messages {
		item := map[string]any{"role": normalizeRole(msg.Role)}
		attachImages := !imagesAttached && len(req.Images) > 0 && msg.Role == "user" && i == len(req.Messages)-1
		if attachImages {
			parts := make([]map[string]any, 0, 1+len(req.Images))
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, map[string]any{"type": "text", "text": msg.Content})
			}
			for _, image := range req.Images {
				if strings.TrimSpace(image.URL) != "" {
					parts = append(parts, map[string]any{
						"type": "image",
						"source": map[string]string{
							"type":       "url",
							"url":        strings.TrimSpace(image.URL),
							"media_type": normalizeMime(image.MimeType),
						},
					})
					continue
				}
				b64 := strings.TrimSpace(image.Base64)
				if b64 == "" {
					continue
				}
				if _, err := base64.StdEncoding.DecodeString(b64); err != nil {
					continue
				}
				parts = append(parts, map[string]any{
					"type": "image",
					"source": map[string]string{
						"type":       "base64",
						"media_type": normalizeMime(image.MimeType),
						"data":       b64,
					},
				})
			}
			item["content"] = parts
			imagesAttached = true
		} else {
			item["content"] = msg.Content
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			content := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				var args any
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					args = map[string]any{}
				}
				content = append(content, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": args,
				})
			}
			item["content"] = content
		}
		if msg.Role == "tool" {
			var args any
			_ = json.Unmarshal([]byte(msg.Content), &args)
			item["role"] = "user"
			item["content"] = []map[string]any{{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     args,
			}}
		}
		out = append(out, item)
	}
	return out
}

func toAnthropicTools(tools []llm.ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		if strings.TrimSpace(t.Name) == "" {
			continue
		}
		out = append(out, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.Parameters,
		})
	}
	return out
}

func normalizeMime(mime string) string {
	mime = strings.TrimSpace(mime)
	if mime == "" {
		return "image/png"
	}
	return mime
}

func normalizeRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "assistant" {
		return "assistant"
	}
	return "user"
}

func normalizeFinish(reason string) string {
	switch strings.TrimSpace(reason) {
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
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

func maxTokens(v int) int {
	if v > 0 {
		return v
	}
	return 1024
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
