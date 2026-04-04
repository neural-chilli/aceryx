package ollama

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

func TestChat_JSONFormat(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["format"] != "json" {
			t.Fatalf("expected format=json, got %#v", payload["format"])
		}
		_, _ = w.Write([]byte(`{"model":"llama3","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	adapter := New(Config{BaseURL: srv.URL, DefaultModel: "llama3", HTTPClient: srv.Client()})
	resp, err := adapter.Chat(context.Background(), llm.ChatRequest{
		JSONMode: true,
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if resp.Content == "" {
		t.Fatalf("expected response content")
	}
}
