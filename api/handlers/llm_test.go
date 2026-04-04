package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLLMHandlers_Unauthenticated(t *testing.T) {
	h := NewLLMAdminHandlers(nil, nil)

	tests := []struct {
		name string
		fn   http.HandlerFunc
	}{
		{name: "list", fn: h.ListProviders},
		{name: "create", fn: h.CreateProvider},
		{name: "update", fn: h.UpdateProvider},
		{name: "delete", fn: h.DeleteProvider},
		{name: "test", fn: h.TestProvider},
		{name: "usage", fn: h.UsageSummary},
		{name: "details", fn: h.UsageDetails},
		{name: "by-purpose", fn: h.UsageByPurpose},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/admin/llm", nil)
			tt.fn(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rr.Code)
			}
		})
	}
}
