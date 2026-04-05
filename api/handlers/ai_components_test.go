package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIComponentHandlersUnauthenticated(t *testing.T) {
	h := NewAIComponentHandlers(nil)
	tests := []struct {
		name string
		fn   http.HandlerFunc
	}{
		{name: "list", fn: h.List},
		{name: "get", fn: h.Get},
		{name: "create", fn: h.Create},
		{name: "update", fn: h.Update},
		{name: "delete", fn: h.Delete},
		{name: "reload", fn: h.Reload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/ai-components", nil)
			tt.fn(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rr.Code)
			}
		})
	}
}
