package triggers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/plugins"
)

func TestHealthMonitorRestartsErroredTrigger(t *testing.T) {
	tenantID := uuid.New()
	rt := &mockRuntime{plugin: triggerPluginForTests(&plugins.TriggerContract{Delivery: "at_least_once", State: "host_managed", Concurrency: "single", Ordering: "ordered"})}
	m := NewTriggerManager(rt, nil, nil, &mockPipeline{}, newMemoryStore(), TriggerManagerConfig{MaxRestartBackoff: time.Millisecond})
	if err := m.Start(context.Background(), uuid.New(), plugins.PluginRef{ID: "test-trigger"}, triggerConfigJSON(tenantID, "")); err != nil {
		t.Fatalf("start: %v", err)
	}
	item := m.List()[0]
	inst, err := m.getInstance(item.ID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	inst.setStatus(TriggerError, "panic")

	hm := NewHealthMonitor(m, 10*time.Millisecond)
	hm.Check(context.Background())
	if !waitUntil(time.Second, func() bool {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		return rt.startCalls >= 2
	}) {
		t.Fatal("expected restart call")
	}
}
