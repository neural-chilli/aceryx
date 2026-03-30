package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateOutputAgainstSchema(t *testing.T) {
	min := 0.0
	max := 100.0
	schema := map[string]FieldDef{
		"score":      {Type: "number", Min: &min, Max: &max},
		"risk_level": {Type: "string", Enum: []any{"low", "high"}},
		"confidence": {Type: "number", Min: ptrFloat(0), Max: ptrFloat(1)},
	}

	valid := map[string]any{"score": 42.0, "risk_level": "low", "confidence": 0.9}
	if err := validateOutputAgainstSchema(valid, schema); err != nil {
		t.Fatalf("expected valid output, got error %v", err)
	}

	invalidType := map[string]any{"score": "nope", "risk_level": "low", "confidence": 0.9}
	if err := validateOutputAgainstSchema(invalidType, schema); err == nil {
		t.Fatal("expected type mismatch error")
	}

	missing := map[string]any{"score": 42.0, "risk_level": "low"}
	if err := validateOutputAgainstSchema(missing, schema); err == nil {
		t.Fatal("expected missing field error")
	}
}

func TestRenderPromptTemplate_Functions(t *testing.T) {
	raw := `Risk {{.case.risk}} amount {{formatCurrency .case.amount}} date {{formatDate .now}} json {{toJSON .steps}}`
	input := map[string]any{
		"case":  map[string]any{"risk": "low", "amount": 50000},
		"steps": map[string]any{"credit": map[string]any{"result": "ok"}},
		"now":   "2026-03-29T12:00:00Z",
	}
	out, err := renderPromptTemplate(raw, input)
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}
	if out == "" {
		t.Fatal("expected rendered prompt")
	}
}

func TestParseTemplateReference(t *testing.T) {
	name, version := parseTemplateReference("risk_assessment_v3")
	if name != "risk_assessment" || version != 3 {
		t.Fatalf("unexpected parse result: %s %d", name, version)
	}
	name, version = parseTemplateReference("risk_assessment")
	if name != "risk_assessment" || version != 0 {
		t.Fatalf("unexpected parse fallback: %s %d", name, version)
	}
}

func TestLLMClientChatCompletion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "test-model",
				"choices": []map[string]any{{
					"finish_reason": "stop",
					"message":       map[string]any{"content": `{"ok":true,"confidence":0.9}`},
				}},
				"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
			})
		}))
		defer srv.Close()

		c := NewLLMClient(srv.URL, "test-model", "", time.Second)
		resp, err := c.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
		if err != nil {
			t.Fatalf("chat completion: %v", err)
		}
		if resp.Content == "" {
			t.Fatal("expected content")
		}
	})

	t.Run("http_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad"))
		}))
		defer srv.Close()

		c := NewLLMClient(srv.URL, "test-model", "", time.Second)
		if _, err := c.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("malformed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}))
		defer srv.Close()

		c := NewLLMClient(srv.URL, "test-model", "", time.Second)
		if _, err := c.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err == nil {
			t.Fatal("expected decode error")
		}
	})
}

func ptrFloat(v float64) *float64 { return &v }
