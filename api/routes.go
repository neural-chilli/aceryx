package api

import (
	"database/sql"
	"net/http"
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
	h := handlers.NewCaseHandlers(ctSvc, caseSvc, reportSvc)
	rbacSvc := rbac.NewService(db)

	wrap := func(permission string, fn http.HandlerFunc) http.Handler {
		return middleware.Auth(middleware.RequirePermission(rbacSvc, permission)(http.HandlerFunc(fn)))
	}

	mux.Handle("POST /case-types", wrap("cases:create", h.RegisterCaseType))
	mux.Handle("GET /case-types", wrap("cases:read", h.ListCaseTypes))
	mux.Handle("GET /case-types/{id}", wrap("cases:read", h.GetCaseType))

	mux.Handle("POST /cases", wrap("cases:create", h.CreateCase))
	mux.Handle("GET /cases/{id}", wrap("cases:read", h.GetCase))
	mux.Handle("GET /cases", wrap("cases:read", h.ListCases))
	mux.Handle("PATCH /cases/{id}/data", wrap("cases:update", h.PatchCaseData))
	mux.Handle("POST /cases/{id}/close", wrap("cases:close", h.CloseCase))
	mux.Handle("POST /cases/{id}/cancel", wrap("cases:close", h.CancelCase))
	mux.Handle("GET /cases/search", wrap("cases:read", h.SearchCases))
	mux.Handle("GET /cases/dashboard", wrap("cases:read", h.Dashboard))

	mux.Handle("GET /reports/cases/summary", wrap("cases:read", h.ReportCasesSummary))
	mux.Handle("GET /reports/cases/ageing", wrap("cases:read", h.ReportAgeing))
	mux.Handle("GET /reports/sla/compliance", wrap("cases:read", h.ReportSLACompliance))
	mux.Handle("GET /reports/cases/by-stage", wrap("cases:read", h.ReportCasesByStage))
	mux.Handle("GET /reports/workload", wrap("cases:read", h.ReportWorkload))
	mux.Handle("GET /reports/decisions", wrap("cases:read", h.ReportDecisions))

	return mux
}
