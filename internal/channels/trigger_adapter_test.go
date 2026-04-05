package channels

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/triggers"
)

func TestTriggerPipelineAdapterRoutesThroughUnifiedPipeline(t *testing.T) {
	t.Parallel()

	store := &fakeChannelStore{
		channel: &Channel{
			ID:            uuid.New(),
			TenantID:      uuid.New(),
			Type:          ChannelPlugin,
			Enabled:       true,
			CaseTypeID:    uuid.New(),
			AdapterConfig: AdapterConfig{},
		},
		cases: map[uuid.UUID]fakeCaseRecord{},
	}
	pipeline := NewPipeline(noopWorkflowRunner{}, store, nil)
	adapter := NewTriggerPipelineAdapter(pipeline)

	result, err := adapter.Process(context.Background(), triggers.PipelineRequest{
		TenantID:  store.channel.TenantID,
		ChannelID: store.channel.ID,
		Data:      []byte(`{"reference":"TR-1"}`),
		Source:    "trigger",
	})
	if err != nil {
		t.Fatalf("adapter process: %v", err)
	}
	if result.CaseID == uuid.Nil || result.EventID == uuid.Nil {
		t.Fatalf("expected case and event IDs from unified pipeline")
	}
	if len(store.events) != 1 || store.events[0].Status != EventProcessed {
		t.Fatalf("expected processed channel event, got %#v", store.events)
	}
}
