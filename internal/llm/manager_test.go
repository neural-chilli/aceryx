package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeAdapter struct {
	provider  string
	chatFn    func(context.Context, ChatRequest) (ChatResponse, error)
	embedFn   func(context.Context, []string, string) ([][]float32, error)
	chatCalls int
}

func (f *fakeAdapter) Provider() string { return f.provider }
func (f *fakeAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	f.chatCalls++
	if f.chatFn != nil {
		return f.chatFn(ctx, req)
	}
	return ChatResponse{}, nil
}
func (f *fakeAdapter) Embed(ctx context.Context, texts []string, model string) ([][]float32, error) {
	if f.embedFn != nil {
		return f.embedFn(ctx, texts, model)
	}
	return nil, nil
}
func (f *fakeAdapter) SupportsVision() bool { return false }
func (f *fakeAdapter) SupportsJSON() bool   { return false }
func (f *fakeAdapter) Models() []ModelInfo  { return nil }
func (f *fakeAdapter) Close() error         { return nil }

type fakeStore struct {
	invocations []Invocation
}

func (s *fakeStore) RecordInvocation(_ context.Context, inv Invocation) error {
	s.invocations = append(s.invocations, inv)
	return nil
}
func (s *fakeStore) GetMonthlyUsage(context.Context, uuid.UUID, string) (MonthlyUsage, error) {
	return MonthlyUsage{}, nil
}
func (s *fakeStore) UpdateMonthlyUsage(context.Context, uuid.UUID, int, float64) error {
	return nil
}
func (s *fakeStore) ListInvocations(context.Context, uuid.UUID, ListOpts) ([]Invocation, error) {
	return nil, nil
}
func (s *fakeStore) UsageByPurpose(context.Context, uuid.UUID, time.Time) ([]PurposeUsage, error) {
	return nil, nil
}
func (s *fakeStore) ListProviders(context.Context, uuid.UUID) ([]LLMProviderConfig, error) {
	return nil, nil
}
func (s *fakeStore) GetProvider(context.Context, uuid.UUID, uuid.UUID) (LLMProviderConfig, error) {
	return LLMProviderConfig{}, errors.New("not implemented")
}
func (s *fakeStore) CreateProvider(context.Context, LLMProviderConfig) (LLMProviderConfig, error) {
	return LLMProviderConfig{}, errors.New("not implemented")
}
func (s *fakeStore) UpdateProvider(context.Context, LLMProviderConfig) (LLMProviderConfig, error) {
	return LLMProviderConfig{}, errors.New("not implemented")
}
func (s *fakeStore) DeleteProvider(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func TestManager_ResolveModel(t *testing.T) {
	tenantID := uuid.New()
	manager := NewAdapterManager(nil, nil, func(context.Context, LLMProviderConfig, string) (LLMAdapter, error) {
		return &fakeAdapter{provider: "openai"}, nil
	})
	err := manager.AddProvider(LLMProviderConfig{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Provider:     "openai",
		DefaultModel: "gpt-4o-mini",
		IsDefault:    true,
		ModelMap: map[string]string{
			"small": "gpt-4o-mini",
			"large": "gpt-4o",
		},
	})
	if err != nil {
		t.Fatalf("add provider: %v", err)
	}

	if got := manager.ResolveModel(tenantID, "small"); got != "gpt-4o-mini" {
		t.Fatalf("expected small mapping, got %q", got)
	}
	if got := manager.ResolveModel(tenantID, "large"); got != "gpt-4o" {
		t.Fatalf("expected large mapping, got %q", got)
	}
}

func TestManager_FailoverOnServerError(t *testing.T) {
	tenantID := uuid.New()
	primaryID := uuid.New()
	fallbackID := uuid.New()
	primary := &fakeAdapter{
		provider: "openai",
		chatFn: func(context.Context, ChatRequest) (ChatResponse, error) {
			return ChatResponse{}, &HTTPStatusError{StatusCode: 500, Body: "down"}
		},
	}
	fallback := &fakeAdapter{
		provider: "anthropic",
		chatFn: func(context.Context, ChatRequest) (ChatResponse, error) {
			return ChatResponse{Content: "ok"}, nil
		},
	}
	store := &fakeStore{}
	manager := NewAdapterManager(store, nil, func(_ context.Context, cfg LLMProviderConfig, _ string) (LLMAdapter, error) {
		if cfg.ID == primaryID {
			return primary, nil
		}
		return fallback, nil
	})
	if err := manager.AddProvider(LLMProviderConfig{ID: primaryID, TenantID: tenantID, Provider: "openai", DefaultModel: "gpt-4o-mini", IsDefault: true, Enabled: true}); err != nil {
		t.Fatalf("add primary: %v", err)
	}
	if err := manager.AddProvider(LLMProviderConfig{ID: fallbackID, TenantID: tenantID, Provider: "anthropic", DefaultModel: "claude", IsFallback: true, Enabled: true}); err != nil {
		t.Fatalf("add fallback: %v", err)
	}
	resp, err := manager.Chat(context.Background(), tenantID, ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("expected fallback response, got %q", resp.Content)
	}
	if primary.chatCalls != 1 || fallback.chatCalls != 1 {
		t.Fatalf("expected one call each, got primary=%d fallback=%d", primary.chatCalls, fallback.chatCalls)
	}
}

func TestManager_NoFailoverOnClientError(t *testing.T) {
	tenantID := uuid.New()
	primaryID := uuid.New()
	fallbackID := uuid.New()
	primary := &fakeAdapter{
		provider: "openai",
		chatFn: func(context.Context, ChatRequest) (ChatResponse, error) {
			return ChatResponse{}, &HTTPStatusError{StatusCode: 400, Body: "bad request"}
		},
	}
	fallback := &fakeAdapter{
		provider: "anthropic",
		chatFn: func(context.Context, ChatRequest) (ChatResponse, error) {
			return ChatResponse{Content: "ok"}, nil
		},
	}
	manager := NewAdapterManager(nil, nil, func(_ context.Context, cfg LLMProviderConfig, _ string) (LLMAdapter, error) {
		if cfg.ID == primaryID {
			return primary, nil
		}
		return fallback, nil
	})
	_ = manager.AddProvider(LLMProviderConfig{ID: primaryID, TenantID: tenantID, Provider: "openai", DefaultModel: "gpt-4o-mini", IsDefault: true, Enabled: true})
	_ = manager.AddProvider(LLMProviderConfig{ID: fallbackID, TenantID: tenantID, Provider: "anthropic", DefaultModel: "claude", IsFallback: true, Enabled: true})

	_, err := manager.Chat(context.Background(), tenantID, ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected error")
	}
	if fallback.chatCalls != 0 {
		t.Fatalf("expected no fallback call, got %d", fallback.chatCalls)
	}
}
