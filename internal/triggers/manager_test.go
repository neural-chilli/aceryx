package triggers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/plugins"
)

func TestTriggerManagerLifecycle(t *testing.T) {
	tenantID := uuid.New()
	rt := &mockRuntime{plugin: triggerPluginForTests(&plugins.TriggerContract{Delivery: "at_least_once", State: "host_managed", Concurrency: "single", Ordering: "ordered"})}
	store := newMemoryStore()
	manager := NewTriggerManager(rt, nil, nil, &mockPipeline{}, store, TriggerManagerConfig{GracefulStopTimeout: time.Second})

	if err := manager.Start(context.Background(), uuid.New(), plugins.PluginRef{ID: "test-trigger"}, triggerConfigJSON(tenantID, "")); err != nil {
		t.Fatalf("start: %v", err)
	}
	items := manager.List()
	if len(items) != 1 || items[0].Status != TriggerRunning {
		t.Fatalf("expected one running instance, got %+v", items)
	}
	if err := manager.Stop(context.Background(), items[0].ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	item, err := manager.Get(items[0].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if item.Status != TriggerStopped {
		t.Fatalf("expected stopped, got %s", item.Status)
	}
}

func TestTriggerManagerExactlyOnceValidation(t *testing.T) {
	tenantID := uuid.New()
	rt := &mockRuntime{plugin: triggerPluginForTests(&plugins.TriggerContract{Delivery: "exactly_once", State: "host_managed", Concurrency: "single", Ordering: "ordered"})}
	manager := NewTriggerManager(rt, nil, nil, &mockPipeline{}, newMemoryStore(), TriggerManagerConfig{})
	if err := manager.Start(context.Background(), uuid.New(), plugins.PluginRef{ID: "test-trigger"}, triggerConfigJSON(tenantID, "kafka")); err == nil {
		t.Fatal("expected startup failure without driver registry for exactly_once")
	}
}
