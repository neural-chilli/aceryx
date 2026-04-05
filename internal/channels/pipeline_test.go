package channels

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeWorkflowEngine struct {
	evaluated []uuid.UUID
}

func (f *fakeWorkflowEngine) EvaluateDAG(_ context.Context, caseID uuid.UUID) error {
	f.evaluated = append(f.evaluated, caseID)
	return nil
}

type fakeAttachmentService struct {
	fail bool
}

func (f *fakeAttachmentService) Store(_ context.Context, _ uuid.UUID, _ uuid.UUID, in AttachmentInput) (AttachmentRef, error) {
	if f.fail {
		return AttachmentRef{}, errors.New("store failed")
	}
	return AttachmentRef{
		VaultID:     uuid.New(),
		Filename:    in.Filename,
		ContentType: in.ContentType,
		Size:        int64(len(in.Data)),
		Checksum:    "abc123",
	}, nil
}

type fakeCaseRecord struct {
	id   uuid.UUID
	data map[string]any
}

type fakeChannelStore struct {
	channel      *Channel
	createErr    error
	failedEvents []*ChannelEvent

	cases  map[uuid.UUID]fakeCaseRecord
	events []*ChannelEvent
}

func (s *fakeChannelStore) Create(context.Context, *Channel) error { return nil }
func (s *fakeChannelStore) Update(context.Context, *Channel) error { return nil }
func (s *fakeChannelStore) Get(_ context.Context, tenantID, channelID uuid.UUID) (*Channel, error) {
	if s.channel != nil && s.channel.TenantID == tenantID && s.channel.ID == channelID {
		return s.channel, nil
	}
	return nil, errors.New("not found")
}
func (s *fakeChannelStore) GetByID(_ context.Context, channelID uuid.UUID) (*Channel, error) {
	if s.channel != nil && s.channel.ID == channelID {
		return s.channel, nil
	}
	return nil, errors.New("not found")
}
func (s *fakeChannelStore) List(context.Context, uuid.UUID) ([]*Channel, error) { return nil, nil }
func (s *fakeChannelStore) ListEnabled(context.Context) ([]*Channel, error)     { return nil, nil }
func (s *fakeChannelStore) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *fakeChannelStore) SetEnabled(context.Context, uuid.UUID, uuid.UUID, bool) error {
	return nil
}
func (s *fakeChannelStore) ListEvents(context.Context, uuid.UUID, uuid.UUID, int, int) ([]*ChannelEvent, error) {
	return nil, nil
}
func (s *fakeChannelStore) RecordFailedEvent(_ context.Context, event *ChannelEvent) (uuid.UUID, error) {
	s.failedEvents = append(s.failedEvents, event)
	return uuid.New(), nil
}

func (s *fakeChannelStore) WithTx(ctx context.Context, fn func(txCtx context.Context, tx TxStore) error) error {
	stagingCases := make(map[uuid.UUID]fakeCaseRecord, len(s.cases))
	for k, v := range s.cases {
		stagingCases[k] = v
	}
	stagingEvents := make([]*ChannelEvent, 0, len(s.events))
	for _, evt := range s.events {
		copyEvt := *evt
		stagingEvents = append(stagingEvents, &copyEvt)
	}
	tx := &fakeTxStore{
		parent: s,
		cases:  stagingCases,
		events: stagingEvents,
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	s.cases = tx.cases
	s.events = tx.events
	return nil
}

type fakeTxStore struct {
	parent *fakeChannelStore
	cases  map[uuid.UUID]fakeCaseRecord
	events []*ChannelEvent
}

func (t *fakeTxStore) GetChannel(context.Context, uuid.UUID, uuid.UUID) (*Channel, error) {
	return t.parent.channel, nil
}

func (t *fakeTxStore) FindRecentEvents(_ context.Context, _, _ uuid.UUID, since time.Time) ([]*ChannelEvent, error) {
	out := make([]*ChannelEvent, 0)
	for _, evt := range t.events {
		if evt.CreatedAt.IsZero() || evt.CreatedAt.After(since) {
			out = append(out, evt)
		}
	}
	return out, nil
}

func (t *fakeTxStore) FindCaseByFields(context.Context, uuid.UUID, uuid.UUID, []string, json.RawMessage) (*uuid.UUID, error) {
	return nil, nil
}

func (t *fakeTxStore) CreateCase(_ context.Context, in CreateOrUpdateCaseInput) (uuid.UUID, error) {
	if t.parent.createErr != nil {
		return uuid.Nil, t.parent.createErr
	}
	id := uuid.New()
	t.cases[id] = fakeCaseRecord{id: id, data: in.Data}
	return id, nil
}

func (t *fakeTxStore) UpdateCaseData(_ context.Context, _ uuid.UUID, caseID uuid.UUID, patch map[string]any) error {
	record := t.cases[caseID]
	if record.data == nil {
		record.data = map[string]any{}
	}
	for k, v := range patch {
		record.data[k] = v
	}
	t.cases[caseID] = record
	return nil
}

func (t *fakeTxStore) RecordEvent(_ context.Context, event *ChannelEvent) (uuid.UUID, error) {
	eventID := uuid.New()
	copyEvt := *event
	copyEvt.ID = eventID
	if copyEvt.CreatedAt.IsZero() {
		copyEvt.CreatedAt = time.Now().UTC()
	}
	t.events = append(t.events, &copyEvt)
	return eventID, nil
}

func TestPipelineProcessFullPath(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	channelID := uuid.New()
	workflowID := uuid.New()
	store := &fakeChannelStore{
		channel: &Channel{
			ID:         channelID,
			TenantID:   tenantID,
			Type:       ChannelWebhook,
			Enabled:    true,
			CaseTypeID: uuid.New(),
			WorkflowID: &workflowID,
			AdapterConfig: AdapterConfig{Mappings: []FieldMapping{
				{Type: "direct", Source: "payload.amount", Target: "case.data.loan.amount"},
			}},
			DedupConfig: DedupConfig{Strategy: "field_hash", Fields: []string{"reference"}, WindowMins: 60},
		},
		cases: map[uuid.UUID]fakeCaseRecord{},
	}
	workflow := &fakeWorkflowEngine{}
	pipeline := NewPipeline(workflow, store, &fakeAttachmentService{})

	result, err := pipeline.Process(context.Background(), PipelineRequest{
		TenantID:  tenantID,
		ChannelID: channelID,
		Data:      []byte(`{"amount":50000,"reference":"REF-001"}`),
		Attachments: []AttachmentInput{{
			Filename:    "app.pdf",
			ContentType: "application/pdf",
			Data:        []byte("pdf"),
		}},
		Source: "webhook",
	})
	if err != nil {
		t.Fatalf("process pipeline: %v", err)
	}
	if result.CaseID == uuid.Nil || result.EventID == uuid.Nil {
		t.Fatalf("expected case and event ids")
	}
	if len(store.events) != 1 || store.events[0].Status != EventProcessed {
		t.Fatalf("expected one processed event, got %#v", store.events)
	}
	if len(workflow.evaluated) != 1 || workflow.evaluated[0] != result.CaseID {
		t.Fatalf("expected workflow engine call for case %s", result.CaseID)
	}
}

func TestPipelineProcessDedupSecondEvent(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	channelID := uuid.New()
	store := &fakeChannelStore{
		channel: &Channel{
			ID:            channelID,
			TenantID:      tenantID,
			Type:          ChannelWebhook,
			Enabled:       true,
			CaseTypeID:    uuid.New(),
			AdapterConfig: AdapterConfig{},
			DedupConfig:   DedupConfig{Strategy: "field_hash", Fields: []string{"reference"}, WindowMins: 60},
		},
		cases: map[uuid.UUID]fakeCaseRecord{},
	}
	pipeline := NewPipeline(&fakeWorkflowEngine{}, store, &fakeAttachmentService{})
	req := PipelineRequest{
		TenantID:  tenantID,
		ChannelID: channelID,
		Data:      []byte(`{"reference":"REF-002"}`),
		Source:    "webhook",
	}

	if _, err := pipeline.Process(context.Background(), req); err != nil {
		t.Fatalf("first process failed: %v", err)
	}
	if _, err := pipeline.Process(context.Background(), req); !errors.Is(err, ErrDeduped) {
		t.Fatalf("expected ErrDeduped, got %v", err)
	}
	if len(store.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(store.events))
	}
	if store.events[1].Status != EventDeduped {
		t.Fatalf("expected second event to be deduped, got %s", store.events[1].Status)
	}
}

func TestPipelineProcessFailureRollsBackAndRecordsFailedEvent(t *testing.T) {
	t.Parallel()

	store := &fakeChannelStore{
		channel: &Channel{
			ID:            uuid.New(),
			TenantID:      uuid.New(),
			Type:          ChannelWebhook,
			Enabled:       true,
			CaseTypeID:    uuid.New(),
			AdapterConfig: AdapterConfig{},
			DedupConfig:   DedupConfig{},
		},
		createErr: errors.New("create case failed"),
		cases:     map[uuid.UUID]fakeCaseRecord{},
	}
	pipeline := NewPipeline(&fakeWorkflowEngine{}, store, &fakeAttachmentService{})
	_, err := pipeline.Process(context.Background(), PipelineRequest{
		TenantID:  store.channel.TenantID,
		ChannelID: store.channel.ID,
		Data:      []byte(`{"a":1}`),
		Source:    "webhook",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(store.cases) != 0 || len(store.events) != 0 {
		t.Fatalf("expected tx rollback with no cases/events, got %d cases and %d events", len(store.cases), len(store.events))
	}
	if len(store.failedEvents) != 1 || store.failedEvents[0].Status != EventFailed {
		t.Fatalf("expected one failed event, got %#v", store.failedEvents)
	}
}

func TestPipelineProcessAtomicityOnAttachmentFailure(t *testing.T) {
	t.Parallel()

	store := &fakeChannelStore{
		channel: &Channel{
			ID:            uuid.New(),
			TenantID:      uuid.New(),
			Type:          ChannelWebhook,
			Enabled:       true,
			CaseTypeID:    uuid.New(),
			AdapterConfig: AdapterConfig{},
		},
		cases: map[uuid.UUID]fakeCaseRecord{},
	}
	pipeline := NewPipeline(&fakeWorkflowEngine{}, store, &fakeAttachmentService{fail: true})
	_, err := pipeline.Process(context.Background(), PipelineRequest{
		TenantID:  store.channel.TenantID,
		ChannelID: store.channel.ID,
		Data:      []byte(`{"a":1}`),
		Attachments: []AttachmentInput{{
			Filename: "f.txt", Data: []byte("x"),
		}},
		Source: "form",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(store.cases) != 0 {
		t.Fatalf("expected rollback of created case")
	}
	if len(store.failedEvents) != 1 {
		t.Fatalf("expected failed event after rollback")
	}
}
