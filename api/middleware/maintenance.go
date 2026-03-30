package middleware

import (
	"net/http"
	"os"

	"github.com/neural-chilli/aceryx/internal/backup"
)

func MaintenanceModeMiddleware(next http.Handler) http.Handler {
	vaultPath := firstNonEmpty(os.Getenv("ACERYX_VAULT_PATH"), os.Getenv("ACERYX_VAULT_ROOT"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if backup.IsMaintenanceMode(vaultPath) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"maintenance mode","code":"MAINTENANCE_MODE"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
