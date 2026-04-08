package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractionHandlersUnauthenticated(t *testing.T) {
	h := NewExtractionHandlers(nil)
	tests := []struct {
		name string
		fn   http.HandlerFunc
	}{
		{name: "list", fn: h.ListSchemas},
		{name: "create", fn: h.CreateSchema},
		{name: "get", fn: h.GetSchema},
		{name: "update", fn: h.UpdateSchema},
		{name: "delete", fn: h.DeleteSchema},
		{name: "get-job", fn: h.GetJob},
		{name: "list-fields", fn: h.ListFields},
		{name: "accept-job", fn: h.AcceptJob},
		{name: "reject-job", fn: h.RejectJob},
		{name: "confirm-field", fn: h.ConfirmField},
		{name: "correct-field", fn: h.CorrectField},
		{name: "reject-field", fn: h.RejectField},
		{name: "list-corrections", fn: h.ListCorrections},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/extraction-schemas", nil)
			tt.fn(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rr.Code)
			}
		})
	}
}
