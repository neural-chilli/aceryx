package llm

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ChatRequest struct {
	SystemPrompt string
	Messages     []Message
	Model        string
	MaxTokens    int
	Temperature  float64
	JSONMode     bool
	Images       []ImageInput
	Tools        []ToolDef
	ToolChoice   string
	Purpose      string
}

type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

type ChatResponse struct {
	Content      string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Model        string
	FinishReason string
	ToolCalls    []ToolCall
}

type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ImageInput struct {
	Base64   string
	MimeType string
	URL      string
}

type ModelInfo struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	ContextWindow      int     `json:"context_window"`
	SupportsVision     bool    `json:"supports_vision"`
	SupportsJSON       bool    `json:"supports_json"`
	CostPerInputToken  float64 `json:"cost_per_input_token"`
	CostPerOutputToken float64 `json:"cost_per_output_token"`
}

type ListOpts struct {
	Since  time.Time
	Limit  int
	Offset int
}

type MonthlyUsage struct {
	TenantID        uuid.UUID `json:"tenant_id"`
	YearMonth       string    `json:"year_month"`
	TotalTokens     int64     `json:"total_tokens"`
	TotalCostUSD    float64   `json:"total_cost_usd"`
	InvocationCount int       `json:"invocation_count"`
}

type PurposeUsage struct {
	Purpose         string  `json:"purpose"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalCostUSD    float64 `json:"total_cost_usd"`
	InvocationCount int     `json:"invocation_count"`
}

type Invocation struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	ProviderID   uuid.UUID `json:"provider_id"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Purpose      string    `json:"purpose"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	DurationMS   int       `json:"duration_ms"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message"`
	CostUSD      float64   `json:"cost_usd"`
	CreatedAt    time.Time `json:"created_at"`
}

type LLMProviderConfig struct {
	ID                 uuid.UUID            `json:"id"`
	TenantID           uuid.UUID            `json:"tenant_id"`
	Provider           string               `json:"provider"`
	Name               string               `json:"name"`
	EndpointURL        string               `json:"endpoint_url,omitempty"`
	APIKeySecret       string               `json:"api_key_secret"`
	DefaultModel       string               `json:"default_model"`
	MaxTokens          int                  `json:"max_tokens"`
	Temperature        float64              `json:"temperature"`
	IsDefault          bool                 `json:"is_default"`
	IsFallback         bool                 `json:"is_fallback"`
	Enabled            bool                 `json:"enabled"`
	ModelMap           map[string]string    `json:"model_map"`
	ModelPricing       map[string]ModelInfo `json:"model_pricing"`
	RequestsPerMin     int                  `json:"requests_per_min"`
	TenantRPM          int                  `json:"tenant_requests_per_min"`
	MonthlyTokenBudget int64                `json:"monthly_token_budget"`
	MonthlyCostBudget  float64              `json:"monthly_cost_budget"`
	AzureAPIVersion    string               `json:"azure_api_version,omitempty"`
	AzureDeployment    string               `json:"azure_deployment,omitempty"`
	Azure              bool                 `json:"azure"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

func (c LLMProviderConfig) PricingForModel(model string) ModelInfo {
	if c.ModelPricing == nil {
		return ModelInfo{ID: model}
	}
	if info, ok := c.ModelPricing[model]; ok {
		if info.ID == "" {
			info.ID = model
		}
		return info
	}
	return ModelInfo{ID: model}
}

func cloneModelMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clonePricing(in map[string]ModelInfo) map[string]ModelInfo {
	if len(in) == 0 {
		return map[string]ModelInfo{}
	}
	out := make(map[string]ModelInfo, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeJSONMap(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func normalizePricingMap(raw json.RawMessage) map[string]ModelInfo {
	if len(raw) == 0 {
		return map[string]ModelInfo{}
	}
	out := map[string]ModelInfo{}
	_ = json.Unmarshal(raw, &out)
	return out
}
