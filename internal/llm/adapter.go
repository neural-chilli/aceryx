package llm

import "context"

// LLMAdapter is the Aceryx-owned interface for all LLM providers.
type LLMAdapter interface {
	// Provider returns the provider identifier (e.g. "openai", "anthropic").
	Provider() string

	// Chat sends a chat completion request and returns the response.
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)

	// Embed generates embeddings for a batch of texts.
	Embed(ctx context.Context, texts []string, model string) ([][]float32, error)

	// SupportsVision returns whether this adapter supports image inputs.
	SupportsVision() bool

	// SupportsJSON returns whether this adapter supports native JSON mode.
	SupportsJSON() bool

	// Models returns the list of available models for this provider.
	Models() []ModelInfo

	// Close releases any resources held by the adapter.
	Close() error
}
