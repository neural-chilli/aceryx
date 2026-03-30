package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
	"github.com/neural-chilli/aceryx/internal/rbac"
)

type Principal struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	SessionID *uuid.UUID
	Type      string
	Name      string
	Email     string
	Roles     []string
}

type principalCtxKey struct{}

func PrincipalFromContext(ctx context.Context) *Principal {
	v, _ := ctx.Value(principalCtxKey{}).(*Principal)
	return v
}

func AuthMiddleware(auth *rbac.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := strings.TrimSpace(r.Header.Get("Authorization"))
			if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated"})
				return
			}

			token := strings.TrimSpace(authz[len("Bearer "):])
			ap, err := auth.AuthenticateBearer(r.Context(), token)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated"})
				return
			}

			ctx := context.WithValue(r.Context(), principalCtxKey{}, &Principal{
				ID:        ap.ID,
				TenantID:  ap.TenantID,
				SessionID: ap.SessionID,
				Type:      ap.Type,
				Name:      ap.Name,
				Email:     ap.Email,
				Roles:     ap.Roles,
			})
			ctx = observability.WithTenantID(ctx, ap.TenantID)
			ctx = observability.WithPrincipalID(ctx, ap.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
