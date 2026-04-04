package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLLMClientChatCompletion_ConnectionRefused(t *testing.T) {
	client := NewLLMClient("http://127.0.0.1:1", "test-model", "", 200*time.Millisecond)
	_, err := client.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err == nil {
		t.Fatal("expected connection refused error")
	}
	if !strings.Contains(err.Error(), "chat completion request failed") {
		t.Fatalf("expected wrapped transport failure, got %v", err)
	}
}

func TestLLMClientChatCompletion_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(250 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"finish_reason":"stop","message":{"content":"{}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	client := NewLLMClient(srv.URL, "test-model", "", 50*time.Millisecond)
	_, err := client.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "chat completion request failed") {
		t.Fatalf("expected wrapped timeout error, got %v", err)
	}
}

func TestLLMClientChatCompletion_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"x","choices":[{"message":`))
	}))
	defer srv.Close()

	client := NewLLMClient(srv.URL, "test-model", "", time.Second)
	_, err := client.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err == nil {
		t.Fatal("expected malformed response error")
	}
	if !strings.Contains(err.Error(), "decode chat completion response") {
		t.Fatalf("expected decode failure, got %v", err)
	}
}
