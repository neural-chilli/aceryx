package connectors

import (
	"context"
	"testing"
)

type testConnector struct{}

func (c *testConnector) Meta() ConnectorMeta {
	return ConnectorMeta{Key: "t", Name: "Test"}
}

func (c *testConnector) Auth() AuthSpec { return AuthSpec{Type: "none"} }
func (c *testConnector) Triggers() []TriggerSpec {
	return []TriggerSpec{{Key: "x", Type: "webhook"}}
}
func (c *testConnector) Actions() []ActionSpec {
	return []ActionSpec{{Key: "ping", Execute: func(_ context.Context, _ map[string]string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	}}}
}

func TestRegistry_RegisterGetListAndAction(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&testConnector{})

	if _, ok := reg.Get("t"); !ok {
		t.Fatal("expected connector t to be registered")
	}
	if len(reg.List()) != 1 {
		t.Fatalf("expected one connector in list, got %d", len(reg.List()))
	}
	action, ok := reg.GetAction("t", "ping")
	if !ok {
		t.Fatal("expected action ping")
	}
	res, err := action.Execute(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if got := res["ok"]; got != true {
		t.Fatalf("expected ok=true, got %#v", got)
	}
}
