package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/neural-chilli/aceryx/internal/observability"
)

func RequestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		level := slog.LevelInfo
		if r.URL.Path == "/health" || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			level = slog.LevelDebug
		}
		if duration > time.Second {
			level = slog.LevelWarn
		}

		attrs := append(observability.RequestAttrs(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", duration.Milliseconds(),
		)
		slog.Log(r.Context(), level, "request", attrs...)
	})
}
