package api

import (
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/neural-chilli/aceryx/api/handlers"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/rbac"
)

// NewRouter creates and configures the HTTP router.
func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handlers.Health)
	return mux
}

func NewRouterWithServices(db *sql.DB, eng *engine.Engine) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handlers.Health)

	ctSvc := cases.NewCaseTypeService(db)
	caseSvc := cases.NewCaseService(db, eng)
	reportSvc := cases.NewReportsService(db, 5*time.Minute)
	caseHandlers := handlers.NewCaseHandlers(ctSvc, caseSvc, reportSvc)

	authzSvc := rbac.NewService(db)
	authSvc := rbac.NewAuthService(db, os.Getenv("ACERYX_JWT_SECRET"), parseDurationOrDefault(os.Getenv("ACERYX_SESSION_TTL"), 24*time.Hour))
	principalSvc := rbac.NewPrincipalService(db, authzSvc)
	roleSvc := rbac.NewRoleService(db, authzSvc)
	authHandlers := handlers.NewAuthHandlers(authSvc, principalSvc, roleSvc)

	authMW := middleware.AuthMiddleware(authSvc)
	withAuth := func(h http.HandlerFunc) http.Handler {
		return authMW(http.HandlerFunc(h))
	}
	withPerm := func(permission string, h http.HandlerFunc) http.Handler {
		return authMW(middleware.RequirePermission(authzSvc, authSvc, permission)(http.HandlerFunc(h)))
	}

	mux.HandleFunc("POST /auth/login", authHandlers.Login)
	mux.Handle("POST /auth/logout", withAuth(authHandlers.Logout))
	mux.Handle("POST /auth/password", withAuth(authHandlers.ChangePassword))
	mux.Handle("GET /auth/preferences", withAuth(authHandlers.GetPreferences))
	mux.Handle("PUT /auth/preferences", withAuth(authHandlers.PutPreferences))

	mux.Handle("POST /admin/principals", withPerm("admin:users", authHandlers.CreatePrincipal))
	mux.Handle("GET /admin/principals", withPerm("admin:users", authHandlers.ListPrincipals))
	mux.Handle("PUT /admin/principals/{id}", withPerm("admin:users", authHandlers.UpdatePrincipal))
	mux.Handle("POST /admin/principals/{id}/disable", withPerm("admin:users", authHandlers.DisablePrincipal))

	mux.Handle("POST /admin/roles", withPerm("admin:roles", authHandlers.CreateRole))
	mux.Handle("GET /admin/roles", withPerm("admin:roles", authHandlers.ListRoles))
	mux.Handle("PUT /admin/roles/{id}/permissions", withPerm("admin:roles", authHandlers.UpdateRolePermissions))

	mux.Handle("POST /case-types", withPerm("cases:create", caseHandlers.RegisterCaseType))
	mux.Handle("GET /case-types", withPerm("cases:read", caseHandlers.ListCaseTypes))
	mux.Handle("GET /case-types/{id}", withPerm("cases:read", caseHandlers.GetCaseType))

	mux.Handle("POST /cases", withPerm("cases:create", caseHandlers.CreateCase))
	mux.Handle("GET /cases/{id}", withPerm("cases:read", caseHandlers.GetCase))
	mux.Handle("GET /cases", withPerm("cases:read", caseHandlers.ListCases))
	mux.Handle("PATCH /cases/{id}/data", withPerm("cases:update", caseHandlers.PatchCaseData))
	mux.Handle("POST /cases/{id}/close", withPerm("cases:close", caseHandlers.CloseCase))
	mux.Handle("POST /cases/{id}/cancel", withPerm("cases:close", caseHandlers.CancelCase))
	mux.Handle("GET /cases/search", withPerm("cases:read", caseHandlers.SearchCases))
	mux.Handle("GET /cases/dashboard", withPerm("cases:read", caseHandlers.Dashboard))

	mux.Handle("GET /reports/cases/summary", withPerm("cases:read", caseHandlers.ReportCasesSummary))
	mux.Handle("GET /reports/cases/ageing", withPerm("cases:read", caseHandlers.ReportAgeing))
	mux.Handle("GET /reports/sla/compliance", withPerm("cases:read", caseHandlers.ReportSLACompliance))
	mux.Handle("GET /reports/cases/by-stage", withPerm("cases:read", caseHandlers.ReportCasesByStage))
	mux.Handle("GET /reports/workload", withPerm("cases:read", caseHandlers.ReportWorkload))
	mux.Handle("GET /reports/decisions", withPerm("cases:read", caseHandlers.ReportDecisions))

	return mux
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
