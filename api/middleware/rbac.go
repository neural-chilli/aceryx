package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/neural-chilli/aceryx/internal/rbac"
)

func RequirePermission(authz *rbac.Service, auth *rbac.AuthService, permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := PrincipalFromContext(r.Context())
			if principal == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated"})
				return
			}

			if err := authz.Authorize(r.Context(), principal.ID, permission); err != nil {
				if auth != nil {
					auth.RecordDenied(r.Context(), rbac.AuthPrincipal{ID: principal.ID, TenantID: principal.TenantID}, permission, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
