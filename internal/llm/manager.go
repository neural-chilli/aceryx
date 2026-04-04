package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

const (
	defaultRequestsPerMin = 60
	invocationQueueSize   = 1024
)

type AdapterFactory func(ctx context.Context, config LLMProviderConfig, apiKey string) (LLMAdapter, error)

type SecretResolver interface {
	Get(ctx context.Context, tenantID uuid.UUID, key string) (string, error)
}

type AdapterManager struct {
	mu sync.RWMutex

	adapters  map[uuid.UUID]*managedAdapter
	defaults  map[uuid.UUID]*managedAdapter
	fallbacks map[uuid.UUID]*managedAdapter
	modelMaps map[uuid.UUID]map[string]string

	tenantLimiter map[uuid.UUID]*rate.Limiter
	retryAfter    map[uuid.UUID]time.Time

	store   InvocationStore
	secrets SecretResolver
	factory AdapterFactory

	invocations chan Invocation
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

type managedAdapter struct {
	adapter LLMAdapter
	config  LLMProviderConfig
	limiter *rate.Limiter
}

func NewAdapterManager(store InvocationStore, secrets SecretResolver, factory AdapterFactory) *AdapterManager {
	m := &AdapterManager{
		adapters:      map[uuid.UUID]*managedAdapter{},
		defaults:      map[uuid.UUID]*managedAdapter{},
		fallbacks:     map[uuid.UUID]*managedAdapter{},
		modelMaps:     map[uuid.UUID]map[string]string{},
		tenantLimiter: map[uuid.UUID]*rate.Limiter{},
		retryAfter:    map[uuid.UUID]time.Time{},
		store:         store,
		secrets:       secrets,
		factory:       factory,
		invocations:   make(chan Invocation, invocationQueueSize),
		stopCh:        make(chan struct{}),
	}
	if store != nil {
		m.wg.Add(1)
		go m.runInvocationWorker()
	}
	return m
}

func (m *AdapterManager) Close() error {
	close(m.stopCh)
	m.wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	for id, managed := range m.adapters {
		if managed == nil || managed.adapter == nil {
			continue
		}
		if err := managed.adapter.Close(); err != nil {
			slog.Warn("llm adapter close failed", "provider_id", id.String(), "error", err)
		}
	}
	return nil
}

func (m *AdapterManager) AddProvider(config LLMProviderConfig) error {
	if m == nil {
		return fmt.Errorf("adapter manager is nil")
	}
	if config.ID == uuid.Nil {
		config.ID = uuid.New()
	}
	if config.RequestsPerMin <= 0 {
		config.RequestsPerMin = defaultRequestsPerMin
	}
	if config.TenantRPM <= 0 {
		config.TenantRPM = defaultRequestsPerMin
	}
	apiKey := ""
	if m.secrets != nil && strings.TrimSpace(config.APIKeySecret) != "" {
		resolved, err := m.secrets.Get(context.Background(), config.TenantID, config.APIKeySecret)
		if err != nil {
			return fmt.Errorf("resolve llm provider secret: %w", err)
		}
		apiKey = resolved
	}
	if m.factory == nil {
		return fmt.Errorf("adapter factory is required")
	}
	adapter, err := m.factory(context.Background(), config, apiKey)
	if err != nil {
		return fmt.Errorf("create %s adapter: %w", config.Provider, err)
	}

	managed := &managedAdapter{
		adapter: adapter,
		config:  config,
		limiter: rate.NewLimiter(rate.Limit(float64(config.RequestsPerMin)/60.0), max(1, config.RequestsPerMin/6)),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters[config.ID] = managed
	if config.IsDefault || m.defaults[config.TenantID] == nil {
		m.defaults[config.TenantID] = managed
	}
	if config.IsFallback {
		m.fallbacks[config.TenantID] = managed
	}
	if _, ok := m.modelMaps[config.TenantID]; !ok {
		m.modelMaps[config.TenantID] = map[string]string{}
	}
	for k, v := range config.ModelMap {
		m.modelMaps[config.TenantID][k] = v
	}
	if _, ok := m.tenantLimiter[config.TenantID]; !ok {
		m.tenantLimiter[config.TenantID] = rate.NewLimiter(rate.Limit(float64(config.TenantRPM)/60.0), max(1, config.TenantRPM/6))
	}
	return nil
}

func (m *AdapterManager) RemoveProvider(configID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	managed, ok := m.adapters[configID]
	if !ok {
		return nil
	}
	delete(m.adapters, configID)
	if managed != nil {
		tenantID := managed.config.TenantID
		if d, ok := m.defaults[tenantID]; ok && d == managed {
			delete(m.defaults, tenantID)
		}
		if f, ok := m.fallbacks[tenantID]; ok && f == managed {
			delete(m.fallbacks, tenantID)
		}
		if managed.adapter != nil {
			return managed.adapter.Close()
		}
	}
	return nil
}

func (m *AdapterManager) ResolveModel(tenantID uuid.UUID, hint string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mapping := m.modelMaps[tenantID]
	if mapping == nil {
		return ""
	}
	return strings.TrimSpace(mapping[strings.TrimSpace(strings.ToLower(hint))])
}

func (m *AdapterManager) TestProvider(ctx context.Context, configID uuid.UUID) error {
	m.mu.RLock()
	managed := m.adapters[configID]
	m.mu.RUnlock()
	if managed == nil {
		return fmt.Errorf("provider not found")
	}
	_, err := managed.adapter.Chat(ctx, ChatRequest{
		Messages:  []Message{{Role: "user", Content: "Hello, respond with OK"}},
		Model:     managed.config.DefaultModel,
		MaxTokens: 16,
	})
	return err
}

func (m *AdapterManager) Chat(ctx context.Context, tenantID uuid.UUID, req ChatRequest) (ChatResponse, error) {
	managed, fallback := m.selectAdapters(tenantID)
	if managed == nil {
		return ChatResponse{}, ErrProviderUnavailable
	}
	if req.Model == "" {
		req.Model = managed.config.DefaultModel
	}

	resp, inv, err := m.callChat(ctx, managed, tenantID, req)
	m.enqueueInvocation(inv)
	if err == nil {
		return resp, nil
	}
	if fallback == nil || fallback == managed || IsClientProviderError(err) || !IsRetryableProviderError(err) {
		return ChatResponse{}, err
	}

	fbResp, fbInv, fbErr := m.callChat(ctx, fallback, tenantID, req)
	m.enqueueInvocation(fbInv)
	if fbErr != nil {
		return ChatResponse{}, fbErr
	}
	return fbResp, nil
}

func (m *AdapterManager) Embed(ctx context.Context, tenantID uuid.UUID, texts []string, model string) ([][]float32, error) {
	managed, fallback := m.selectAdapters(tenantID)
	if managed == nil {
		return nil, ErrProviderUnavailable
	}
	if model == "" {
		model = managed.config.DefaultModel
	}

	vectors, inv, err := m.callEmbed(ctx, managed, tenantID, texts, model)
	m.enqueueInvocation(inv)
	if err == nil {
		return vectors, nil
	}
	if fallback == nil || fallback == managed || IsClientProviderError(err) || !IsRetryableProviderError(err) {
		return nil, err
	}

	vectors, inv, err = m.callEmbed(ctx, fallback, tenantID, texts, model)
	m.enqueueInvocation(inv)
	if err != nil {
		return nil, err
	}
	return vectors, nil
}

func (m *AdapterManager) callChat(ctx context.Context, managed *managedAdapter, tenantID uuid.UUID, req ChatRequest) (ChatResponse, Invocation, error) {
	started := time.Now()
	inv := Invocation{
		ID:         uuid.New(),
		TenantID:   tenantID,
		ProviderID: managed.config.ID,
		Provider:   managed.config.Provider,
		Model:      req.Model,
		Purpose:    normalizePurpose(req.Purpose, "assistant"),
		CreatedAt:  started.UTC(),
	}

	if err := m.waitForLimiters(ctx, managed); err != nil {
		inv.Status = "rate_limited"
		inv.DurationMS = int(time.Since(started).Milliseconds())
		inv.ErrorMessage = err.Error()
		return ChatResponse{}, inv, err
	}

	resp, err := managed.adapter.Chat(ctx, req)
	inv.DurationMS = int(time.Since(started).Milliseconds())
	if err != nil {
		inv.Status = statusForError(err)
		inv.ErrorMessage = err.Error()
		if retryAfter, ok := retryAfterFromError(err); ok {
			m.setRetryAfter(managed.config.ID, retryAfter)
		}
		return ChatResponse{}, inv, err
	}

	inv.Status = "success"
	inv.InputTokens = resp.InputTokens
	inv.OutputTokens = resp.OutputTokens
	inv.TotalTokens = resp.TotalTokens
	inv.Model = coalesce(resp.Model, inv.Model)
	inv.CostUSD = estimateCostUSD(managed.config, inv.Model, inv.InputTokens, inv.OutputTokens)
	return resp, inv, nil
}

func (m *AdapterManager) callEmbed(ctx context.Context, managed *managedAdapter, tenantID uuid.UUID, texts []string, model string) ([][]float32, Invocation, error) {
	started := time.Now()
	inv := Invocation{
		ID:         uuid.New(),
		TenantID:   tenantID,
		ProviderID: managed.config.ID,
		Provider:   managed.config.Provider,
		Model:      model,
		Purpose:    "embedding",
		CreatedAt:  started.UTC(),
	}
	if err := m.waitForLimiters(ctx, managed); err != nil {
		inv.Status = "rate_limited"
		inv.DurationMS = int(time.Since(started).Milliseconds())
		inv.ErrorMessage = err.Error()
		return nil, inv, err
	}
	vecs, err := managed.adapter.Embed(ctx, texts, model)
	inv.DurationMS = int(time.Since(started).Milliseconds())
	if err != nil {
		inv.Status = statusForError(err)
		inv.ErrorMessage = err.Error()
		if retryAfter, ok := retryAfterFromError(err); ok {
			m.setRetryAfter(managed.config.ID, retryAfter)
		}
		return nil, inv, err
	}
	inv.Status = "success"
	inv.TotalTokens = 0
	inv.CostUSD = estimateCostUSD(managed.config, inv.Model, inv.InputTokens, inv.OutputTokens)
	return vecs, inv, nil
}

func (m *AdapterManager) waitForLimiters(ctx context.Context, managed *managedAdapter) error {
	if m == nil || managed == nil {
		return fmt.Errorf("provider unavailable")
	}
	m.mu.RLock()
	tenantLimiter := m.tenantLimiter[managed.config.TenantID]
	retryAt := m.retryAfter[managed.config.ID]
	m.mu.RUnlock()

	if !retryAt.IsZero() && time.Now().Before(retryAt) {
		return ErrRateLimited
	}
	if tenantLimiter != nil {
		if err := tenantLimiter.Wait(ctx); err != nil {
			return fmt.Errorf("tenant rate limit wait: %w", err)
		}
	}
	if managed.limiter != nil {
		if err := managed.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("provider rate limit wait: %w", err)
		}
	}
	return nil
}

func (m *AdapterManager) selectAdapters(tenantID uuid.UUID) (primary *managedAdapter, fallback *managedAdapter) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	primary = m.defaults[tenantID]
	fallback = m.fallbacks[tenantID]
	if primary == nil {
		for _, item := range m.adapters {
			if item == nil || item.config.TenantID != tenantID || !item.config.Enabled {
				continue
			}
			primary = item
			break
		}
	}
	if fallback != nil && !fallback.config.Enabled {
		fallback = nil
	}
	if primary != nil && !primary.config.Enabled {
		primary = nil
	}
	return primary, fallback
}

func (m *AdapterManager) enqueueInvocation(inv Invocation) {
	if m == nil || m.store == nil {
		return
	}
	select {
	case m.invocations <- inv:
	default:
		go m.persistInvocation(inv)
	}
}

func (m *AdapterManager) runInvocationWorker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stopCh:
			return
		case inv := <-m.invocations:
			m.persistInvocation(inv)
		}
	}
}

func (m *AdapterManager) persistInvocation(inv Invocation) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.store.RecordInvocation(ctx, inv); err != nil {
		slog.Warn("record llm invocation failed", "error", err)
		return
	}
	if inv.Purpose == "test" {
		return
	}
	if err := m.store.UpdateMonthlyUsage(ctx, inv.TenantID, inv.TotalTokens, inv.CostUSD); err != nil {
		slog.Warn("update llm monthly usage failed", "error", err)
		return
	}
	m.logBudgetWarnings(ctx, inv)
}

func (m *AdapterManager) logBudgetWarnings(ctx context.Context, inv Invocation) {
	m.mu.RLock()
	managed := m.adapters[inv.ProviderID]
	m.mu.RUnlock()
	if managed == nil {
		return
	}
	if managed.config.MonthlyTokenBudget <= 0 && managed.config.MonthlyCostBudget <= 0 {
		return
	}
	usage, err := m.store.GetMonthlyUsage(ctx, inv.TenantID, inv.CreatedAt.Format("2006-01"))
	if err != nil {
		return
	}
	if managed.config.MonthlyTokenBudget > 0 {
		percent := float64(usage.TotalTokens) / float64(managed.config.MonthlyTokenBudget)
		if percent >= 1 {
			slog.Warn("tenant token budget exceeded", "tenant_id", inv.TenantID.String(), "percent", percent)
		} else if percent >= 0.8 {
			slog.Warn("tenant token budget at 80%", "tenant_id", inv.TenantID.String(), "percent", percent)
		}
	}
	if managed.config.MonthlyCostBudget > 0 {
		percent := usage.TotalCostUSD / managed.config.MonthlyCostBudget
		if percent >= 1 {
			slog.Warn("tenant cost budget exceeded", "tenant_id", inv.TenantID.String(), "percent", percent)
		} else if percent >= 0.8 {
			slog.Warn("tenant cost budget at 80%", "tenant_id", inv.TenantID.String(), "percent", percent)
		}
	}
}

func (m *AdapterManager) setRetryAfter(providerID uuid.UUID, t time.Time) {
	if t.IsZero() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryAfter[providerID] = t
}

func estimateCostUSD(config LLMProviderConfig, model string, inputTokens, outputTokens int) float64 {
	pricing := config.PricingForModel(model)
	return (float64(inputTokens) * pricing.CostPerInputToken) + (float64(outputTokens) * pricing.CostPerOutputToken)
}

func retryAfterFromError(err error) (time.Time, bool) {
	if errors.Is(err, ErrRateLimited) {
		return time.Now().Add(10 * time.Second), true
	}
	var hs *HTTPStatusError
	if errors.As(err, &hs) && hs.StatusCode == 429 {
		return time.Now().Add(10 * time.Second), true
	}
	return time.Time{}, false
}

func statusForError(err error) string {
	if errors.Is(err, ErrRateLimited) {
		return "rate_limited"
	}
	var hs *HTTPStatusError
	if errors.As(err, &hs) && hs.StatusCode == 429 {
		return "rate_limited"
	}
	return "error"
}

func normalizePurpose(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	return value
}

func coalesce(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
