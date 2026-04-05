package triggers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestAdminAPIResetCheckpointsConflict(t *testing.T) {
	m := NewTriggerManager(&mockRuntime{plugin: triggerPluginForTests(nil)}, nil, nil, &mockPipeline{}, newMemoryStore(), TriggerManagerConfig{})
	id := uuid.New()
	m.instances[id] = &TriggerInstance{id: id, status: TriggerRunning, checkpointer: newMemoryCheckpointer()}
	h := NewAdminHandlers(m)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/triggers/"+id.String()+"/checkpoints", nil)
	req.SetPathValue("id", id.String())
	w := httptest.NewRecorder()
	h.ResetCheckpoints(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "trigger_running" {
		t.Fatalf("unexpected code: %v", body["code"])
	}
}
