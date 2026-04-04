package custom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neural-chilli/aceryx/internal/llm"
)

func TestInterfaceCompliance(t *testing.T) {
	var _ llm.LLMAdapter = (*Adapter)(nil)
}

func TestAzureHeadersAndAPIVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "secret" {
			t.Fatalf("expected api-key header")
		}
		if r.URL.Query().Get("api-version") != "2024-10-21" {
			t.Fatalf("expected api-version query parameter")
		}
		_, _ = w.Write([]byte(`{"model":"x","choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	adapter := New(Config{
		BaseURL:         srv.URL,
		APIKey:          "secret",
		DefaultModel:    "x",
		Azure:           true,
		AzureAPIVersion: "2024-10-21",
	})
	_, err := adapter.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
}

func TestJSONModeAugmentsPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		msgs, ok := payload["messages"].([]any)
		if !ok || len(msgs) == 0 {
			t.Fatalf("expected messages")
		}
		first, _ := msgs[0].(map[string]any)
		content, _ := first["content"].(string)
		if content == "" {
			t.Fatalf("expected augmented system prompt")
		}
		_, _ = w.Write([]byte(`{"model":"x","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	adapter := New(Config{BaseURL: srv.URL, DefaultModel: "x"})
	_, err := adapter.Chat(context.Background(), llm.ChatRequest{
		JSONMode: true,
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
}
