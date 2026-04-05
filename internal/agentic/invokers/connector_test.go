package invokers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neural-chilli/aceryx/internal/plugins"
)

type pluginRuntimeMock struct{}

func (pluginRuntimeMock) ExecuteStep(context.Context, plugins.PluginRef, plugins.StepInput) (plugins.StepResult, error) {
	return plugins.StepResult{Status: "ok", Output: json.RawMessage(`{"ok":true}`)}, nil
}

func TestConnectorInvoker_Invoke(t *testing.T) {
	inv := NewConnectorInvoker(pluginRuntimeMock{}, "test-plugin", json.RawMessage(`{"x":1}`), 0)
	out, err := inv.Invoke(context.Background(), json.RawMessage(`{"q":"x"}`))
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected payload")
	}
}
