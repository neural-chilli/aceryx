package agentic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

type testInvoker struct{}

func (testInvoker) Invoke(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true}`), nil
}

func TestToolAssemblerFiltersByMode(t *testing.T) {
	ta := NewToolAssembler(nil, nil)
	nodes := []ToolNodeConfig{{ID: "a", Connector: "a"}, {ID: "b", Connector: "b"}}
	manifest, err := ta.Assemble(context.Background(), uuid.New(), ToolPolicy{
		Tools:    []ToolRef{{Ref: "a"}, {Ref: "b"}},
		ToolMode: ToolModeReadOnly,
	}, nodes, func(node ToolNodeConfig, toolName string) (ToolInvoker, string, json.RawMessage, error) {
		safety := "read_only"
		if node.ID == "b" {
			safety = "side_effect"
		}
		return testInvoker{}, safety, []byte(`{"type":"object"}`), nil
	})
	if err != nil {
		t.Fatalf("Assemble error: %v", err)
	}
	if len(manifest.Tools()) != 1 {
		t.Fatalf("expected 1 filtered tool, got %d", len(manifest.Tools()))
	}
}
