package agentic

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAPI_GetEventsFiltered(t *testing.T) {
	tenantID := uuid.New()
	traceID := uuid.New()
	store := &memoryTraceStore{
		trace: &ReasoningTrace{ID: traceID, TenantID: tenantID},
		events: []*ReasoningEvent{
			{EventType: "tool_call"},
			{EventType: "reasoning"},
		},
	}
	api := NewAPI(nil, store)
	events, err := api.GetEvents(context.Background(), tenantID, traceID, "tool_call")
	if err != nil {
		t.Fatalf("GetEvents error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}
