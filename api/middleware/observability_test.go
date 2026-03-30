package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCorrelationIDGeneratedWhenMissing(t *testing.T) {
	h := CorrelationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := observability.CorrelationIDFromContext(r.Context())
		if cid == "" {
			t.Fatal("missing correlation id in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Header().Get(observability.CorrelationHeader) == "" {
		t.Fatal("expected correlation header")
	}
}

func TestCorrelationIDEchoedWhenProvided(t *testing.T) {
	provided := uuid.NewString()
	h := CorrelationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(observability.CorrelationHeader, provided)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got := rr.Header().Get(observability.CorrelationHeader); got != provided {
		t.Fatalf("expected %s, got %s", provided, got)
	}
}

func TestMetricsMiddlewareIncrementsCounterAndDuration(t *testing.T) {
	before := testutil.ToFloat64(observability.HTTPRequestsTotal.WithLabelValues(http.MethodGet, "/metrics-test", "201"))
	beforeDuration := testutil.CollectAndCount(observability.HTTPRequestDurationSeconds)

	h := MetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(r.URL.Path))
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics-test", nil))

	after := testutil.ToFloat64(observability.HTTPRequestsTotal.WithLabelValues(http.MethodGet, "/metrics-test", "201"))
	afterDuration := testutil.CollectAndCount(observability.HTTPRequestDurationSeconds)

	if after <= before {
		t.Fatalf("expected request counter to increase, before=%f after=%f", before, after)
	}
	if afterDuration <= beforeDuration {
		t.Fatalf("expected duration histogram count to increase, before=%d after=%d", beforeDuration, afterDuration)
	}
}

func TestRequestLoggingJSONAndWarnForSlowRequest(t *testing.T) {
	buf := &bytes.Buffer{}
	observability.SetupLogger(buf, slog.LevelDebug)

	h := RequestLoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	h = CorrelationMiddleware(h)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	h.ServeHTTP(rr, req)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("expected log output")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &payload); err != nil {
		t.Fatalf("expected valid json log line: %v", err)
	}
	if payload["msg"] != "request" {
		t.Fatalf("unexpected msg: %v", payload["msg"])
	}
	if payload["level"] != "WARN" {
		t.Fatalf("expected WARN level for slow request, got %v", payload["level"])
	}
	if payload["correlation_id"] == "" {
		t.Fatal("expected correlation_id in log output")
	}
}

func TestRequestLoggingDoesNotLogSecrets(t *testing.T) {
	buf := &bytes.Buffer{}
	observability.SetupLogger(buf, slog.LevelDebug)
	body := strings.NewReader(`{"email":"a@b.com","password":"supersecret","api_key":"key-123"}`)

	h := CorrelationMiddleware(RequestLoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	req = req.WithContext(observability.WithTenantID(context.Background(), uuid.New()))
	h.ServeHTTP(rr, req)

	out := buf.String()
	if strings.Contains(out, "supersecret") || strings.Contains(out, "key-123") {
		t.Fatalf("expected secrets to be absent from logs: %s", out)
	}
}
