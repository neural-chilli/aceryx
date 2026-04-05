package form

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/channels"
)

type fakeChannelStore struct {
	channel *channels.Channel
}

func (s *fakeChannelStore) Create(context.Context, *channels.Channel) error { return nil }
func (s *fakeChannelStore) Update(context.Context, *channels.Channel) error { return nil }
func (s *fakeChannelStore) Get(context.Context, uuid.UUID, uuid.UUID) (*channels.Channel, error) {
	return s.channel, nil
}
func (s *fakeChannelStore) GetByID(context.Context, uuid.UUID) (*channels.Channel, error) {
	return s.channel, nil
}
func (s *fakeChannelStore) List(context.Context, uuid.UUID) ([]*channels.Channel, error) {
	return nil, nil
}
func (s *fakeChannelStore) ListEnabled(context.Context) ([]*channels.Channel, error)     { return nil, nil }
func (s *fakeChannelStore) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error       { return nil }
func (s *fakeChannelStore) SetEnabled(context.Context, uuid.UUID, uuid.UUID, bool) error { return nil }
func (s *fakeChannelStore) ListEvents(context.Context, uuid.UUID, uuid.UUID, int, int) ([]*channels.ChannelEvent, error) {
	return nil, nil
}
func (s *fakeChannelStore) RecordFailedEvent(context.Context, *channels.ChannelEvent) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (s *fakeChannelStore) WithTx(ctx context.Context, fn func(txCtx context.Context, tx channels.TxStore) error) error {
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

type fakeCaseTypeStore struct{}

func (f *fakeCaseTypeStore) GetFormSchema(context.Context, uuid.UUID, uuid.UUID) (json.RawMessage, error) {
	return json.RawMessage(`{"type":"object"}`), nil
}

func newFormHandler(rateLimit int, captcha bool) *FormHandler {
	cfgRaw, _ := json.Marshal(channels.FormConfig{
		RateLimitPerMinute: rateLimit,
		CaptchaEnabled:     captcha,
		CaptchaProvider:    "hcaptcha",
		CaptchaSecret:      "secret",
	})
	channel := &channels.Channel{
		ID:            uuid.New(),
		TenantID:      uuid.New(),
		Type:          channels.ChannelForm,
		Enabled:       true,
		Config:        cfgRaw,
		CaseTypeID:    uuid.New(),
		AdapterConfig: channels.AdapterConfig{},
	}
	store := &fakeChannelStore{channel: channel}
	pipeline := channels.NewPipeline(noopWorkflow{}, store, nil)
	return NewFormHandler(store, pipeline, &fakeCaseTypeStore{})
}

func TestFormRateLimitBlocks11thRequest(t *testing.T) {
	t.Parallel()
	h := newFormHandler(10, false)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/intake/"+h.ChannelStore.(*fakeChannelStore).channel.ID.String(), strings.NewReader(`{"ref":"A"}`))
		req.SetPathValue("channel_id", h.ChannelStore.(*fakeChannelStore).channel.ID.String())
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.1"
		rec := httptest.NewRecorder()
		h.SubmitForm(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d (%s)", i+1, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/intake/"+h.ChannelStore.(*fakeChannelStore).channel.ID.String(), strings.NewReader(`{"ref":"A"}`))
	req.SetPathValue("channel_id", h.ChannelStore.(*fakeChannelStore).channel.ID.String())
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1"
	rec := httptest.NewRecorder()
	h.SubmitForm(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("11th request expected 429, got %d", rec.Code)
	}
}

func TestFormCaptchaInvalidReturns400(t *testing.T) {
	t.Parallel()
	h := newFormHandler(10, true)
	req := httptest.NewRequest(http.MethodPost, "/intake/"+h.ChannelStore.(*fakeChannelStore).channel.ID.String(), strings.NewReader(`{"ref":"A"}`))
	req.SetPathValue("channel_id", h.ChannelStore.(*fakeChannelStore).channel.ID.String())
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.2"
	rec := httptest.NewRecorder()
	h.SubmitForm(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing/invalid captcha token, got %d", rec.Code)
	}
}
