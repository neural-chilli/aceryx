package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type Principal struct {
	ID       uuid.UUID
	TenantID uuid.UUID
}

type principalCtxKey struct{}

func PrincipalFromContext(ctx context.Context) *Principal {
	v, _ := ctx.Value(principalCtxKey{}).(*Principal)
	return v
}

// Auth extracts principal identity from request headers.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := r.Header.Get("X-Principal-ID")
		tid := r.Header.Get("X-Tenant-ID")
		if pid != "" && tid != "" {
			principalID, perr := uuid.Parse(pid)
			tenantID, terr := uuid.Parse(tid)
			if perr == nil && terr == nil {
				ctx := context.WithValue(r.Context(), principalCtxKey{}, &Principal{ID: principalID, TenantID: tenantID})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
