package channels

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

type dedupTxStore struct {
	events       []*ChannelEvent
	matchCaseID  *uuid.UUID
	matchCaseErr error
}

func (d *dedupTxStore) GetChannel(context.Context, uuid.UUID, uuid.UUID) (*Channel, error) {
	return nil, nil
}

func (d *dedupTxStore) FindRecentEvents(_ context.Context, _, _ uuid.UUID, _ time.Time) ([]*ChannelEvent, error) {
	return d.events, nil
}

func (d *dedupTxStore) FindCaseByFields(context.Context, uuid.UUID, uuid.UUID, []string, json.RawMessage) (*uuid.UUID, error) {
	return d.matchCaseID, d.matchCaseErr
}

func (d *dedupTxStore) CreateCase(context.Context, CreateOrUpdateCaseInput) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (d *dedupTxStore) UpdateCaseData(context.Context, uuid.UUID, uuid.UUID, map[string]any) error {
	return nil
}

func (d *dedupTxStore) RecordEvent(context.Context, *ChannelEvent) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func TestDedupFieldHash(t *testing.T) {
	t.Parallel()

	channelID := uuid.New()
	tenantID := uuid.New()
	raw := json.RawMessage(`{"message_id":"m-1"}`)
	tx := &dedupTxStore{events: []*ChannelEvent{{RawPayload: raw}}}
	dc := NewDedupChecker()

	isDup, _, err := dc.Check(context.Background(), tx, tenantID, uuid.New(), channelID, DedupConfig{
		Strategy: "field_hash",
		Fields:   []string{"message_id"},
	}, raw)
	if err != nil {
		t.Fatalf("dedup field hash: %v", err)
	}
	if !isDup {
		t.Fatalf("expected duplicate")
	}
}

func TestDedupTimeWindow(t *testing.T) {
	t.Parallel()

	channelID := uuid.New()
	tenantID := uuid.New()
	tx := &dedupTxStore{events: []*ChannelEvent{{RawPayload: json.RawMessage(`{"reference":"A-1"}`)}}}
	dc := NewDedupChecker()

	isDup, _, err := dc.Check(context.Background(), tx, tenantID, uuid.New(), channelID, DedupConfig{
		Strategy:   "time_window",
		Fields:     []string{"reference"},
		WindowMins: 10,
	}, json.RawMessage(`{"reference":"A-1"}`))
	if err != nil {
		t.Fatalf("dedup time_window: %v", err)
	}
	if !isDup {
		t.Fatalf("expected duplicate")
	}
}

func TestDedupCaseMatch(t *testing.T) {
	t.Parallel()

	match := uuid.New()
	tx := &dedupTxStore{matchCaseID: &match}
	dc := NewDedupChecker()

	isDup, caseID, err := dc.Check(context.Background(), tx, uuid.New(), uuid.New(), uuid.New(), DedupConfig{
		Strategy: "case_match",
		Fields:   []string{"reference"},
	}, json.RawMessage(`{"reference":"REF-1"}`))
	if err != nil {
		t.Fatalf("dedup case_match: %v", err)
	}
	if isDup {
		t.Fatalf("case_match should not mark dedup true")
	}
	if caseID == nil || *caseID != match {
		t.Fatalf("expected matching case id, got %v", caseID)
	}
}
