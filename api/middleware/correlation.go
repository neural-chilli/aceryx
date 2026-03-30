package middleware

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func CorrelationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := strings.TrimSpace(r.Header.Get(observability.CorrelationHeader))
		if correlationID == "" {
			correlationID = uuid.NewString()
		}
		w.Header().Set(observability.CorrelationHeader, correlationID)
		ctx := observability.WithCorrelationID(r.Context(), correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
