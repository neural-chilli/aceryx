package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsEndpointReturnsPrometheusTextFormat(t *testing.T) {
	router := NewRouter()
	// Prime one HTTP metric sample.
	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("expected text/plain content type, got %q", rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(rr.Body.String(), "aceryx_http_requests_total") {
		t.Fatalf("expected prometheus output to include aceryx_http_requests_total, got: %s", rr.Body.String())
	}
}
