package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkflowHandlersUnauthenticated(t *testing.T) {
	h := NewWorkflowHandlers(nil)
	tests := []struct {
		name string
		fn   http.HandlerFunc
		path string
	}{
		{name: "list", fn: h.List, path: "/workflows"},
		{name: "create", fn: h.Create, path: "/workflows"},
		{name: "get_draft", fn: h.GetDraft, path: "/workflows/123/versions/draft"},
		{name: "put_draft", fn: h.PutDraft, path: "/workflows/123/versions/draft"},
		{name: "publish", fn: h.Publish, path: "/workflows/123/publish"},
		{name: "yaml_latest", fn: h.ExportYAMLLatest, path: "/workflows/123/yaml/latest"},
		{name: "yaml_version", fn: h.ExportYAMLVersion, path: "/workflows/123/yaml/1"},
		{name: "yaml_import", fn: h.ImportYAMLDraft, path: "/workflows/123/yaml/draft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			tt.fn(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rr.Code)
			}
		})
	}
}
