package openai

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

func TestChat_JSONModeAndTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		rf, ok := body["response_format"].(map[string]any)
		if !ok || rf["type"] != "json_object" {
			t.Fatalf("expected json response_format, got %#v", body["response_format"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"gpt-4o-mini",
			"choices":[{"finish_reason":"tool_calls","message":{"content":"{\"ok\":true}","tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":"{\"id\":1}"}}]}}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`))
	}))
	defer srv.Close()

	adapter := New(Config{BaseURL: srv.URL, DefaultModel: "gpt-4o-mini"})
	resp, err := adapter.Chat(context.Background(), llm.ChatRequest{
		JSONMode: true,
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
		Tools:    []llm.ToolDef{{Name: "lookup", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("unexpected finish reason %q", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "lookup" {
		t.Fatalf("unexpected tool calls %#v", resp.ToolCalls)
	}
}
