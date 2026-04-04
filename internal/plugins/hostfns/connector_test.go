package hostfns

import (
	"context"
	"testing"

	"github.com/neural-chilli/aceryx/internal/connectors"
)

type testConnector struct{}

func (t *testConnector) Meta() connectors.ConnectorMeta {
	return connectors.ConnectorMeta{Key: "x", Name: "X"}
}

func (t *testConnector) Auth() connectors.AuthSpec { return connectors.AuthSpec{Type: "none"} }
func (t *testConnector) Triggers() []connectors.TriggerSpec {
	return nil
}
func (t *testConnector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{{
		Key: "lookup",
		Execute: func(context.Context, map[string]string, map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	}}
}

func TestCallConnector(t *testing.T) {
	reg := connectors.NewRegistry()
	reg.Register(&testConnector{})
	c := &ConnectorCaller{Registry: reg}

	out, err := c.CallConnector("x", "lookup", map[string]any{})
	if err != nil {
		t.Fatalf("CallConnector error: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestCallConnectorNotFound(t *testing.T) {
	c := &ConnectorCaller{Registry: connectors.NewRegistry()}
	_, err := c.CallConnector("missing", "lookup", map[string]any{})
	if err == nil {
		t.Fatal("expected not found error")
	}
}
