package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssistantHandlersUnauthenticated(t *testing.T) {
	h := NewAssistantHandlers(nil)
	tests := []struct {
		name string
		fn   http.HandlerFunc
		path string
	}{
		{name: "stream", fn: h.Stream, path: "/api/v1/assistant/stream"},
		{name: "message", fn: h.Message, path: "/api/v1/assistant/message"},
		{name: "create_session", fn: h.CreateSession, path: "/api/v1/assistant/sessions"},
		{name: "get_session", fn: h.GetSession, path: "/api/v1/assistant/sessions/123"},
		{name: "delete_session", fn: h.DeleteSession, path: "/api/v1/assistant/sessions/123"},
		{name: "apply_diff", fn: h.ApplyDiff, path: "/api/v1/assistant/diffs/123/apply"},
		{name: "reject_diff", fn: h.RejectDiff, path: "/api/v1/assistant/diffs/123/reject"},
		{name: "list_diffs", fn: h.ListDiffs, path: "/api/v1/assistant/diffs?workflow_id=123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			tt.fn(rr, req)
			if rr.Code != http.StatusUnauthorized && tt.name != "stream" {
				t.Fatalf("expected 401, got %d", rr.Code)
			}
			if tt.name == "stream" && rr.Code != http.StatusNotImplemented {
				t.Fatalf("expected 501, got %d", rr.Code)
			}
		})
	}
}
