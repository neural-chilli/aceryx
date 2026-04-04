package plugins

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestParsePluginRef(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantID  string
		wantVer string
		wantErr bool
	}{
		{name: "latest", in: "slack", wantID: "slack"},
		{name: "pinned", in: "slack@1.0.0", wantID: "slack", wantVer: "1.0.0"},
		{name: "empty", in: "", wantErr: true},
		{name: "missing-version", in: "slack@", wantID: "slack", wantErr: true},
		{name: "invalid-format", in: "a@b@c", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParsePluginRef(tc.in)
			if got.ID != tc.wantID || got.Version != tc.wantVer {
				t.Fatalf("ParsePluginRef(%q) = %#v, want id=%q version=%q", tc.in, got, tc.wantID, tc.wantVer)
			}
			_, err := ParsePluginRefStrict(tc.in)
			if tc.wantErr && err == nil {
				t.Fatalf("expected strict parse error for %q", tc.in)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected strict parse error for %q: %v", tc.in, err)
			}
		})
	}
}

func TestRuntimeLoadAndResolveVersions(t *testing.T) {
	rt := NewRuntime(context.Background(), RuntimeConfig{})
	base := filepath.Join("..", "..", "testdata", "test-plugin")
	if _, err := rt.Load(base, AllowAllLicence{}); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	latest, err := rt.Get(PluginRef{ID: "test-plugin"})
	if err != nil {
		t.Fatalf("Get latest failed: %v", err)
	}
	if latest.Version != "1.0.0" {
		t.Fatalf("unexpected latest version: %s", latest.Version)
	}
	_, err = rt.Get(PluginRef{ID: "test-plugin", Version: "9.9.9"})
	if err == nil {
		t.Fatal("expected not loaded error for missing pinned version")
	}
}

func TestRuntimeExecuteStep(t *testing.T) {
	rt := NewRuntime(context.Background(), RuntimeConfig{})
	base := filepath.Join("..", "..", "testdata", "test-plugin")
	if _, err := rt.Load(base, AllowAllLicence{}); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	out, err := rt.ExecuteStep(context.Background(), PluginRef{ID: "test-plugin"}, StepInput{
		TenantID: uuid.New(),
		Data:     json.RawMessage(`{"hello":"world"}`),
	})
	if err != nil {
		t.Fatalf("ExecuteStep failed: %v", err)
	}
	if out.Status != "ok" {
		t.Fatalf("unexpected status: %s", out.Status)
	}
}

func TestValidateHostFunctionCall(t *testing.T) {
	rt := NewRuntime(context.Background(), RuntimeConfig{})
	p := &Plugin{
		ID:      "test-plugin",
		Version: "1.0.0",
		Manifest: PluginManifest{
			HostFunctions: []string{"host_http_request", "host_log"},
		},
	}
	if err := rt.validateHostFunctionCall(p, "host_http_request"); err != nil {
		t.Fatalf("expected declared function to pass, got %v", err)
	}
	if err := rt.validateHostFunctionCall(p, "host_secret_get"); err == nil || err.Error() != "undeclared host function: host_secret_get" {
		t.Fatalf("expected undeclared function error, got %v", err)
	}
}
