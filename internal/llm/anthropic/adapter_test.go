package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neural-chilli/aceryx/internal/llm"
)

func TestInterfaceCompliance(t *testing.T) {
	var _ llm.LLMAdapter = (*Adapter)(nil)
}

func TestChat_JSONPromptAugmentAndToolParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		system, _ := payload["system"].(string)
		if !strings.Contains(system, "Return only valid JSON") {
			t.Fatalf("expected json instruction in system prompt: %q", system)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"claude-sonnet-4-20250514",
			"stop_reason":"tool_use",
			"content":[
				{"type":"text","text":"{\"ok\":true}"},
				{"type":"tool_use","id":"t1","name":"lookup","input":{"id":1}}
			],
			"usage":{"input_tokens":9,"output_tokens":6}
		}`))
	}))
	defer srv.Close()

	adapter := New(Config{BaseURL: srv.URL, DefaultModel: "claude-sonnet-4-20250514"})
	resp, err := adapter.Chat(context.Background(), llm.ChatRequest{
		JSONMode: true,
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
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

func TestEmbedNotSupported(t *testing.T) {
	adapter := New(Config{})
	_, err := adapter.Embed(context.Background(), []string{"a"}, "m")
	if err == nil {
		t.Fatalf("expected error")
	}
}
