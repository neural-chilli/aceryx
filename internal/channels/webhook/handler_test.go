package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/channels"
)

type fakeSecretStore struct {
	values map[string]string
}

func (s *fakeSecretStore) Get(_ context.Context, _ uuid.UUID, key string) (string, error) {
	return s.values[key], nil
}

func (s *fakeSecretStore) Set(context.Context, uuid.UUID, string, string) error { return nil }
func (s *fakeSecretStore) Delete(context.Context, uuid.UUID, string) error      { return nil }
func (s *fakeSecretStore) List(context.Context, uuid.UUID) ([]string, error)    { return nil, nil }
func (s *fakeSecretStore) ListPrefixed(context.Context, uuid.UUID, string) ([]string, error) {
	return nil, nil
}
func (s *fakeSecretStore) Rotate(context.Context, uuid.UUID, string, string) error { return nil }

type fakeStore struct {
	channel *channels.Channel
}

func (s *fakeStore) Create(context.Context, *channels.Channel) error { return nil }
func (s *fakeStore) Update(context.Context, *channels.Channel) error { return nil }
func (s *fakeStore) Get(context.Context, uuid.UUID, uuid.UUID) (*channels.Channel, error) {
	return s.channel, nil
}
func (s *fakeStore) GetByID(context.Context, uuid.UUID) (*channels.Channel, error) {
	return s.channel, nil
}
func (s *fakeStore) List(context.Context, uuid.UUID) ([]*channels.Channel, error) { return nil, nil }
func (s *fakeStore) ListEnabled(context.Context) ([]*channels.Channel, error)     { return nil, nil }
func (s *fakeStore) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error       { return nil }
func (s *fakeStore) SetEnabled(context.Context, uuid.UUID, uuid.UUID, bool) error { return nil }
func (s *fakeStore) ListEvents(context.Context, uuid.UUID, uuid.UUID, int, int) ([]*channels.ChannelEvent, error) {
	return nil, nil
}
func (s *fakeStore) RecordFailedEvent(context.Context, *channels.ChannelEvent) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (s *fakeStore) WithTx(ctx context.Context, fn func(txCtx context.Context, tx channels.TxStore) error) error {
	return fn(ctx, &fakeTx{channel: s.channel})
}

type fakeTx struct {
	channel *channels.Channel
}

func (t *fakeTx) GetChannel(context.Context, uuid.UUID, uuid.UUID) (*channels.Channel, error) {
	return t.channel, nil
}
func (t *fakeTx) FindRecentEvents(context.Context, uuid.UUID, uuid.UUID, time.Time) ([]*channels.ChannelEvent, error) {
	return nil, nil
}
func (t *fakeTx) FindCaseByFields(context.Context, uuid.UUID, uuid.UUID, []string, json.RawMessage) (*uuid.UUID, error) {
	return nil, nil
}
func (t *fakeTx) CreateCase(context.Context, channels.CreateOrUpdateCaseInput) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (t *fakeTx) UpdateCaseData(context.Context, uuid.UUID, uuid.UUID, map[string]any) error {
	return nil
}
func (t *fakeTx) RecordEvent(context.Context, *channels.ChannelEvent) (uuid.UUID, error) {
	return uuid.New(), nil
}

type noopWorkflow struct{}

func (noopWorkflow) EvaluateDAG(context.Context, uuid.UUID) error { return nil }

func newWebhookHandlerForTest(config channels.WebhookConfig, enabled bool) *WebhookHandler {
	configRaw, _ := json.Marshal(config)
	channel := &channels.Channel{
		ID:            uuid.New(),
		TenantID:      uuid.New(),
		Type:          channels.ChannelWebhook,
		Enabled:       enabled,
		Config:        configRaw,
		CaseTypeID:    uuid.New(),
		AdapterConfig: channels.AdapterConfig{},
		DedupConfig:   channels.DedupConfig{},
	}
	store := &fakeStore{channel: channel}
	pipeline := channels.NewPipeline(noopWorkflow{}, store, nil)
	return &WebhookHandler{
		ChannelStore: store,
		Pipeline:     pipeline,
		SecretStore:  &fakeSecretStore{values: map[string]string{"secret-ref": "super-secret"}},
	}
}

func TestWebhookHMACValidReturns200(t *testing.T) {
	t.Parallel()

	h := newWebhookHandlerForTest(channels.WebhookConfig{
		AuthType:   "hmac",
		AuthSecret: "secret-ref",
		AuthHeader: "X-Signature",
	}, true)
	body := `{"reference":"A-1"}`
	mac := hmac.New(sha256.New, []byte("super-secret"))
	_, _ = mac.Write([]byte(body))
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/webhook/"+h.ChannelStore.(*fakeStore).channel.ID.String(), strings.NewReader(body))
	req.SetPathValue("channel_id", h.ChannelStore.(*fakeStore).channel.ID.String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", "sha256="+sig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWebhookHMACInvalidReturns401(t *testing.T) {
	t.Parallel()
	h := newWebhookHandlerForTest(channels.WebhookConfig{
		AuthType:   "hmac",
		AuthSecret: "secret-ref",
		AuthHeader: "X-Signature",
	}, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/webhook/"+h.ChannelStore.(*fakeStore).channel.ID.String(), strings.NewReader(`{"reference":"A-1"}`))
	req.SetPathValue("channel_id", h.ChannelStore.(*fakeStore).channel.ID.String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", "sha256=deadbeef")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWebhookAPIKeyValidReturns200(t *testing.T) {
	t.Parallel()
	h := newWebhookHandlerForTest(channels.WebhookConfig{
		AuthType:   "api_key",
		AuthSecret: "secret-ref",
		AuthHeader: "X-API-Key",
	}, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/webhook/"+h.ChannelStore.(*fakeStore).channel.ID.String(), strings.NewReader(`{"reference":"A-1"}`))
	req.SetPathValue("channel_id", h.ChannelStore.(*fakeStore).channel.ID.String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "super-secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestWebhookDisabledChannelReturns503(t *testing.T) {
	t.Parallel()
	h := newWebhookHandlerForTest(channels.WebhookConfig{AuthType: "none"}, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/webhook/"+h.ChannelStore.(*fakeStore).channel.ID.String(), strings.NewReader(`{"reference":"A-1"}`))
	req.SetPathValue("channel_id", h.ChannelStore.(*fakeStore).channel.ID.String())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestWebhookNoneAuthReturns200(t *testing.T) {
	t.Parallel()
	h := newWebhookHandlerForTest(channels.WebhookConfig{AuthType: "none"}, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/webhook/"+h.ChannelStore.(*fakeStore).channel.ID.String(), strings.NewReader(`{"reference":"A-1"}`))
	req.SetPathValue("channel_id", h.ChannelStore.(*fakeStore).channel.ID.String())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
