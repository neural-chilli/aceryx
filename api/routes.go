package api

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/handlers"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/agents"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/connectors"
	"github.com/neural-chilli/aceryx/internal/connectors/docgenconn"
	"github.com/neural-chilli/aceryx/internal/connectors/emailconn"
	"github.com/neural-chilli/aceryx/internal/connectors/gchatconn"
	"github.com/neural-chilli/aceryx/internal/connectors/httpconn"
	"github.com/neural-chilli/aceryx/internal/connectors/jiraconn"
	"github.com/neural-chilli/aceryx/internal/connectors/slackconn"
	"github.com/neural-chilli/aceryx/internal/connectors/teamsconn"
	"github.com/neural-chilli/aceryx/internal/connectors/webhookreceiver"
	"github.com/neural-chilli/aceryx/internal/connectors/webhooksender"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/notify"
	"github.com/neural-chilli/aceryx/internal/rbac"
	"github.com/neural-chilli/aceryx/internal/tasks"
	"github.com/neural-chilli/aceryx/internal/tenants"
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
	secretStore := connectors.NewChainedSecretStore(connectors.NewDBSecretStore(db), &connectors.EnvSecretStore{})
	connectorRegistry := connectors.NewRegistry()
	connectorRegistry.Register(httpconn.New())
	connectorRegistry.Register(webhookreceiver.New())
	connectorRegistry.Register(webhooksender.New())
	connectorRegistry.Register(emailconn.New())
	connectorRegistry.Register(slackconn.New())
	connectorRegistry.Register(teamsconn.New())
	connectorRegistry.Register(gchatconn.New())
	connectorRegistry.Register(jiraconn.New())
	connectorRegistry.Register(docgenconn.New(db, nil))
	connectorHandlers := handlers.NewConnectorHandlers(connectorRegistry, secretStore)
	webhookHandler := webhookreceiver.NewHandler(db, secretStore)
	wsHub := notify.NewHub(db, notify.DefaultTokenValidator(func(ctx context.Context, token string) (uuid.UUID, uuid.UUID, error) {
		ap, err := authSvc.AuthenticateBearer(ctx, token)
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
		return ap.ID, ap.TenantID, nil
	}))
	notifySvc := notify.NewService(db, wsHub)
	taskSvc := tasks.NewTaskService(db, eng, notifySvc)
	taskHandlers := handlers.NewTaskHandlers(taskSvc)
	promptTemplateSvc := agents.NewPromptTemplateService(db)
	promptTemplateHandlers := handlers.NewPromptTemplateHandlers(promptTemplateSvc)
	if eng != nil {
		eng.RegisterExecutor("human_task", tasks.NewHumanTaskExecutor(taskSvc))
		eng.RegisterExecutor("integration", connectors.NewExecutor(db, connectorRegistry, secretStore))
		eng.RegisterExecutor("agent", agents.NewAgentExecutor(agents.ExecutorConfig{
			DB:          db,
			TaskCreator: taskSvc,
			LLMClient:   agents.NewLLMClientFromEnv(120 * time.Second),
		}))
		eng.SetEscalationCallback(taskSvc.HandleOverdue)
	}
	tenantSvc := tenants.NewTenantService(db)
	themeSvc := tenants.NewThemeService(db)
	tenantHandlers := handlers.NewTenantHandlers(tenantSvc, themeSvc)

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

	mux.HandleFunc("GET /tenant/branding", tenantHandlers.GetBranding)
	mux.Handle("PUT /tenant/branding", withPerm("admin:tenant", tenantHandlers.PutBranding))
	mux.Handle("GET /tenant/terminology", withAuth(tenantHandlers.GetTerminology))
	mux.Handle("PUT /tenant/terminology", withPerm("admin:tenant", tenantHandlers.PutTerminology))
	mux.Handle("GET /tenant/themes", withAuth(tenantHandlers.ListThemes))
	mux.Handle("POST /tenant/themes", withPerm("admin:tenant", tenantHandlers.CreateTheme))
	mux.Handle("PUT /tenant/themes/{id}", withPerm("admin:tenant", tenantHandlers.UpdateTheme))
	mux.Handle("DELETE /tenant/themes/{id}", withPerm("admin:tenant", tenantHandlers.DeleteTheme))

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
	mux.Handle("GET /connectors", withAuth(connectorHandlers.List))
	mux.Handle("POST /connectors/{key}/actions/{action}/test", withPerm("workflows:edit", connectorHandlers.TestAction))
	mux.Handle("GET /prompt-templates", withPerm("workflows:view", promptTemplateHandlers.List))
	mux.Handle("POST /prompt-templates", withPerm("workflows:edit", promptTemplateHandlers.Create))
	mux.Handle("GET /prompt-templates/{name}/versions/{version}", withPerm("workflows:view", promptTemplateHandlers.GetVersion))
	mux.Handle("PUT /prompt-templates/{name}", withPerm("workflows:edit", promptTemplateHandlers.Update))
	mux.Handle("POST /webhooks/{path...}", http.HandlerFunc(webhookHandler.ServeHTTP))
	mux.Handle("GET /tasks", withAuth(taskHandlers.Inbox))
	mux.Handle("GET /tasks/{case_id}/{step_id}", withAuth(taskHandlers.GetTask))
	mux.Handle("POST /tasks/{case_id}/{step_id}/claim", withAuth(taskHandlers.Claim))
	mux.Handle("POST /tasks/{case_id}/{step_id}/complete", withAuth(taskHandlers.Complete))
	mux.Handle("PUT /tasks/{case_id}/{step_id}/draft", withAuth(taskHandlers.SaveDraft))
	mux.Handle("POST /tasks/{case_id}/{step_id}/reassign", withPerm("tasks:reassign", taskHandlers.Reassign))
	mux.Handle("POST /tasks/{case_id}/{step_id}/escalate", withPerm("tasks:escalate", taskHandlers.Escalate))
	mux.HandleFunc("GET /ws", wsHub.HandleWS)

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
