package triggers

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestResetCheckpointsRequiresStopped(t *testing.T) {
	m := NewTriggerManager(&mockRuntime{plugin: triggerPluginForTests(nil)}, nil, nil, &mockPipeline{}, newMemoryStore(), TriggerManagerConfig{})
	id := uuid.New()
	cp := newMemoryCheckpointer()
	cp.entries["k"] = "v"
	m.instances[id] = &TriggerInstance{id: id, status: TriggerRunning, checkpointer: cp}
	if err := m.ResetCheckpoints(context.Background(), id); err == nil {
		t.Fatal("expected conflict-style error")
	}
	m.instances[id].status = TriggerStopped
	if err := m.ResetCheckpoints(context.Background(), id); err != nil {
		t.Fatalf("reset checkpoints: %v", err)
	}
	if len(cp.entries) != 0 {
		t.Fatal("expected checkpoints cleared")
	}
}
