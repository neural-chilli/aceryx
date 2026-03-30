package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ResponseFormat struct {
	Type       string         `json:"type"`
	JSONSchema map[string]any `json:"json_schema,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"prompt_tokens"`
	OutputTokens int `json:"completion_tokens"`
}

type ChatResponse struct {
	Content      string
	Model        string
	Usage        Usage
	FinishReason string
}

type LLMClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

func NewLLMClient(endpoint, model, apiKey string, timeout time.Duration) *LLMClient {
	if timeout <= 0 {
		timeout = defaultLLMTimeout
	}
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	return &LLMClient{
		endpoint: endpoint,
		model:    model,
		apiKey:   apiKey,
		client:   &http.Client{Timeout: timeout},
	}
}

func NewLLMClientFromEnv(timeout time.Duration) *LLMClient {
	return NewLLMClient(
		strings.TrimSpace(os.Getenv("ACERYX_LLM_ENDPOINT")),
		strings.TrimSpace(os.Getenv("ACERYX_LLM_MODEL")),
		strings.TrimSpace(os.Getenv("ACERYX_LLM_API_KEY")),
		timeout,
	)
}

func (c *LLMClient) ChatCompletion(ctx context.Context, messages []Message, responseFormat *ResponseFormat) (*ChatResponse, error) {
	if c == nil || c.endpoint == "" {
		return nil, fmt.Errorf("llm endpoint not configured")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages are required")
	}

	payload := map[string]any{
		"model":    c.model,
		"messages": messages,
	}
	if responseFormat != nil {
		payload["response_format"] = responseFormat
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal chat payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chat completion request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chat completion response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chat completion non-200: %d %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("decode chat completion response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("chat completion response has no choices")
	}
	return &ChatResponse{
		Content:      decoded.Choices[0].Message.Content,
		Model:        decoded.Model,
		Usage:        decoded.Usage,
		FinishReason: decoded.Choices[0].FinishReason,
	}, nil
}

func (c *LLMClient) Embed(ctx context.Context, input string) ([]float32, error) {
	if c == nil || c.endpoint == "" {
		return nil, fmt.Errorf("llm endpoint not configured")
	}
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("embedding input is required")
	}
	embeddingModel := strings.TrimSpace(os.Getenv("ACERYX_EMBEDDING_MODEL"))
	if embeddingModel == "" {
		embeddingModel = c.model
	}
	if embeddingModel == "" {
		embeddingModel = "text-embedding-3-small"
	}

	payload := map[string]any{"model": embeddingModel, "input": input}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal embeddings payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embeddings response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings non-200: %d %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("decode embeddings response: %w", err)
	}
	if len(decoded.Data) == 0 {
		return nil, fmt.Errorf("embeddings response has no data")
	}
	return decoded.Data[0].Embedding, nil
}
