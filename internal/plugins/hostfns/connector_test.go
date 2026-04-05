package hostfns

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/neural-chilli/aceryx/internal/connectors"
	httpfw "github.com/neural-chilli/aceryx/internal/http"
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

type resolverFunc func(tenantID, pluginID, connectorID, operation string, input map[string]any) (*HTTPConnectorSpec, error)

func (f resolverFunc) ResolveHTTPConnector(tenantID, pluginID, connectorID, operation string, input map[string]any) (*HTTPConnectorSpec, error) {
	return f(tenantID, pluginID, connectorID, operation, input)
}

func TestCallConnectorHTTPPlumbing(t *testing.T) {
	cm := httpfw.NewClientManager(httpfw.ClientConfig{SystemMaxTimeout: 2 * time.Second})
	cm.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("X-From") != "resolver" {
				t.Fatalf("expected resolver header")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		}),
	})
	validator := httpfw.NewURLValidator(true)
	cm.SetValidator(validator)

	caller := &ConnectorCaller{
		ClientManager: cm,
		TenantID:      "t1",
		PluginID:      "p1",
		Resolver: resolverFunc(func(_, _, connectorID, operation string, _ map[string]any) (*HTTPConnectorSpec, error) {
			if connectorID != "companies-house" || operation != "lookup" {
				return nil, nil
			}
			return &HTTPConnectorSpec{
				Request: httpfw.PluginHTTPRequest{
					Method:  http.MethodGet,
					URL:     "https://93.184.216.34/lookup",
					Headers: map[string]string{"X-From": "resolver"},
				},
				ParseResponse: func(resp httpfw.PluginHTTPResponse) (map[string]any, error) {
					return map[string]any{"status": resp.Status}, nil
				},
			}, nil
		}),
	}

	out, err := caller.CallConnector("companies-house", "lookup", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("CallConnector error: %v", err)
	}
	if out["status"] != 200 {
		t.Fatalf("unexpected output: %#v", out)
	}
}
