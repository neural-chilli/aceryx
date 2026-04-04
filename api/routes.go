package api

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/handlers"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/activity"
	"github.com/neural-chilli/aceryx/internal/agents"
	"github.com/neural-chilli/aceryx/internal/audit"
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
	"github.com/neural-chilli/aceryx/internal/llm"
	"github.com/neural-chilli/aceryx/internal/llm/anthropic"
	"github.com/neural-chilli/aceryx/internal/llm/custom"
	"github.com/neural-chilli/aceryx/internal/llm/ollama"
	llmopenai "github.com/neural-chilli/aceryx/internal/llm/openai"
	"github.com/neural-chilli/aceryx/internal/notify"
	"github.com/neural-chilli/aceryx/internal/plugins"
	"github.com/neural-chilli/aceryx/internal/plugins/hostfns"
	"github.com/neural-chilli/aceryx/internal/rbac"
	"github.com/neural-chilli/aceryx/internal/reports"
	"github.com/neural-chilli/aceryx/internal/tasks"
	"github.com/neural-chilli/aceryx/internal/tenants"
	"github.com/neural-chilli/aceryx/internal/vault"
)

// NewRouter creates and configures the HTTP router.
func NewRouter() http.Handler {
	mux := http.NewServeMux()
	health := handlers.NewHealthHandlers(nil, nil, nil)
	mux.HandleFunc("GET /health", health.Health)
	mux.HandleFunc("GET /healthz", health.Liveness)
	mux.HandleFunc("GET /readyz", health.Readiness)
	mux.Handle("GET /metrics", health.Metrics())
	return chainMiddlewares(mux)
}

func NewRouterWithServices(db *sql.DB, eng *engine.Engine) http.Handler {
	return NewRouterWithServicesContext(context.Background(), db, eng)
}

func NewRouterWithServicesContext(bgCtx context.Context, db *sql.DB, eng *engine.Engine) http.Handler {
	mux := http.NewServeMux()

	ctSvc := cases.NewCaseTypeService(db)
	caseSvc := cases.NewCaseService(db, eng)
	reportSvc := cases.NewReportsService(db, 5*time.Minute)
	caseHandlers := handlers.NewCaseHandlers(ctSvc, caseSvc, reportSvc)
	reportingSvc := reports.NewService(db, agents.NewLLMClientFromEnv(120*time.Second))
	reportsHandlers := handlers.NewReportsHandlers(reportingSvc)
	auditSvc := audit.NewService(db)
	auditHandlers := handlers.NewAuditHandlers(auditSvc)

	authzSvc := rbac.NewService(db)
	jwtSecret := firstNonEmpty(os.Getenv("ACERYX_JWT_SECRET"), "test-secret")
	authSvc := rbac.NewAuthService(db, jwtSecret, parseDurationOrDefault(os.Getenv("ACERYX_SESSION_TTL"), 24*time.Hour))
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
	llmStore := llm.NewStore(db)
	llmManager := llm.NewAdapterManager(llmStore, secretStore, func(_ context.Context, config llm.LLMProviderConfig, apiKey string) (llm.LLMAdapter, error) {
		switch strings.TrimSpace(strings.ToLower(config.Provider)) {
		case "openai":
			return llmopenai.New(llmopenai.Config{
				APIKey:       apiKey,
				BaseURL:      config.EndpointURL,
				DefaultModel: config.DefaultModel,
			}), nil
		case "anthropic":
			return anthropic.New(anthropic.Config{
				APIKey:       apiKey,
				BaseURL:      config.EndpointURL,
				DefaultModel: config.DefaultModel,
			}), nil
		case "ollama":
			return ollama.New(ollama.Config{
				BaseURL:      config.EndpointURL,
				DefaultModel: config.DefaultModel,
			}), nil
		case "custom":
			return custom.New(custom.Config{
				APIKey:          apiKey,
				BaseURL:         config.EndpointURL,
				DefaultModel:    config.DefaultModel,
				Azure:           config.Azure,
				AzureAPIVersion: config.AzureAPIVersion,
			}), nil
		default:
			return nil, fmt.Errorf("unsupported llm provider %q", config.Provider)
		}
	})
	llmHandlers := handlers.NewLLMAdminHandlers(llmStore, llmManager)
	pluginStore := plugins.NewStore(db)
	httpHost := hostfns.NewHTTPHost(&http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}, nil, 60*time.Second)
	hostRegistry := &hostfns.Registry{
		HTTP:      httpHost,
		Connector: &hostfns.ConnectorCaller{Registry: connectorRegistry},
		Secrets:   &hostfns.SecretGetter{Store: secretStore},
		Logger:    hostfns.LoggerHost{},
		Auditor:   hostfns.NewAuditor("summary", 50, 10),
	}
	pluginRuntime := plugins.NewRuntime(bgCtx, plugins.RuntimeConfig{
		Store:                pluginStore,
		HostFunctions:        hostRegistry,
		SystemMaxHTTPTimeout: 60 * time.Second,
	})
	pluginHandlers := handlers.NewPluginHandlers(pluginRuntime, pluginStore)
	pluginsDir := firstNonEmpty(os.Getenv("ACERYX_PLUGINS_DIR"), "./testdata")
	_ = pluginRuntime.LoadAll(pluginsDir, plugins.AllowAllLicence{})
	webhookHandler := webhookreceiver.NewHandler(db, secretStore)
	wsHub := notify.NewHub(db, notify.DefaultTokenValidator(func(ctx context.Context, token string) (uuid.UUID, uuid.UUID, error) {
		ap, err := authSvc.AuthenticateBearer(ctx, token)
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
		return ap.ID, ap.TenantID, nil
	}))
	notifySvc := notify.NewService(db, wsHub)
	health := handlers.NewHealthHandlers(db, eng, wsHub)
	activitySvc := activity.NewService(db, wsHub)
	auditSvc.OnCommitted(activitySvc.OnAuditEvent)
	activityHandlers := handlers.NewActivityHandlers(activitySvc)
	taskSvc := tasks.NewTaskService(db, eng, notifySvc)
	taskSvc.SetAuditService(auditSvc)
	caseSvc.SetNotifier(notifySvc)
	caseSvc.SetAuditService(auditSvc)
	taskHandlers := handlers.NewTaskHandlers(taskSvc)
	promptTemplateSvc := agents.NewPromptTemplateService(db)
	promptTemplateHandlers := handlers.NewPromptTemplateHandlers(promptTemplateSvc)
	if eng != nil {
		eng.RegisterExecutor("human_task", tasks.NewHumanTaskExecutor(taskSvc))
		eng.RegisterExecutor("integration", connectors.NewExecutor(db, connectorRegistry, secretStore))
		eng.RegisterExecutor("agent", agents.NewAgentExecutor(agents.ExecutorConfig{
			DB:           db,
			TaskCreator:  taskSvc,
			LLMClient:    agents.NewLLMClientFromEnv(120 * time.Second),
			AuditService: auditSvc,
		}))
		eng.RegisterExecutor("plugin", plugins.NewStepExecutor(db, pluginRuntime))
		eng.SetEscalationCallback(taskSvc.HandleOverdue)
	}
	tenantSvc := tenants.NewTenantService(db)
	themeSvc := tenants.NewThemeService(db)
	tenantHandlers := handlers.NewTenantHandlers(tenantSvc, themeSvc)
	vaultStore := vault.NewLocalVaultStore(os.Getenv("ACERYX_VAULT_ROOT"), firstNonEmpty(os.Getenv("ACERYX_VAULT_SIGNING_KEY"), os.Getenv("ACERYX_JWT_SECRET")))
	vaultSvc := vault.NewService(db, vaultStore, parseDurationOrDefault(os.Getenv("ACERYX_VAULT_CLEANUP_INTERVAL"), 24*time.Hour))
	vaultSvc.SetAuditService(auditSvc)
	vaultHandlers := handlers.NewVaultHandlers(vaultSvc)
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	if shouldStartBackgroundTickers() {
		go vaultSvc.StartOrphanCleanupTicker(bgCtx)
		go reportingSvc.StartViewRefreshTicker(bgCtx)
		go reportingSvc.StartScheduleTicker(bgCtx)
	}

	authMW := middleware.AuthMiddleware(authSvc)
	withAuth := func(h http.HandlerFunc) http.Handler {
		return authMW(http.HandlerFunc(h))
	}
	withPerm := func(permission string, h http.HandlerFunc) http.Handler {
		return authMW(middleware.RequirePermission(authzSvc, authSvc, permission)(http.HandlerFunc(h)))
	}

	mux.HandleFunc("GET /health", health.Health)
	mux.HandleFunc("GET /healthz", health.Liveness)
	mux.HandleFunc("GET /readyz", health.Readiness)
	mux.Handle("GET /metrics", health.Metrics())

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
	mux.Handle("GET /cases/{id}/events", withPerm("cases:read", auditHandlers.ListCaseEvents))
	mux.Handle("GET /activity", withAuth(activityHandlers.Feed))
	mux.Handle("POST /cases/{id}/events/verify", withPerm("admin:audit", auditHandlers.VerifyCaseEvents))
	mux.Handle("GET /cases/{id}/events/export", withPerm("admin:audit", auditHandlers.ExportCaseEvents))
	mux.Handle("POST /cases/{case_id}/documents", withPerm("vault:upload", vaultHandlers.Upload))
	mux.Handle("GET /cases/{case_id}/documents", withPerm("vault:download", vaultHandlers.List))
	mux.Handle("GET /cases/{case_id}/documents/{doc_id}", withPerm("vault:download", vaultHandlers.Download))
	mux.Handle("GET /cases/{case_id}/documents/{doc_id}/signed-url", withPerm("vault:download", vaultHandlers.SignedURL))
	mux.Handle("DELETE /cases/{case_id}/documents/{doc_id}", withPerm("vault:delete", vaultHandlers.Delete))
	mux.Handle("GET /cases/search", withPerm("cases:read", caseHandlers.SearchCases))
	mux.Handle("GET /cases/dashboard", withPerm("cases:read", caseHandlers.Dashboard))

	mux.Handle("GET /reports/cases/summary", withPerm("cases:read", caseHandlers.ReportCasesSummary))
	mux.Handle("GET /reports/cases/ageing", withPerm("cases:read", caseHandlers.ReportAgeing))
	mux.Handle("GET /reports/sla/compliance", withPerm("cases:read", caseHandlers.ReportSLACompliance))
	mux.Handle("GET /reports/cases/by-stage", withPerm("cases:read", caseHandlers.ReportCasesByStage))
	mux.Handle("GET /reports/workload", withPerm("cases:read", caseHandlers.ReportWorkload))
	mux.Handle("GET /reports/decisions", withPerm("cases:read", caseHandlers.ReportDecisions))
	mux.Handle("POST /reports/ask", withPerm("reports:query", reportsHandlers.Ask))
	mux.Handle("POST /reports", withPerm("reports:query", reportsHandlers.Create))
	mux.Handle("GET /reports", withPerm("reports:query", reportsHandlers.List))
	mux.Handle("GET /reports/{id}", withPerm("reports:query", reportsHandlers.Get))
	mux.Handle("POST /reports/{id}/run", withPerm("reports:query", reportsHandlers.Run))
	mux.Handle("PUT /reports/{id}", withPerm("reports:query", reportsHandlers.Update))
	mux.Handle("DELETE /reports/{id}", withPerm("reports:query", reportsHandlers.Delete))
	mux.Handle("POST /admin/erasure", withPerm("admin:audit", vaultHandlers.Erasure))
	mux.Handle("GET /admin/plugins", withPerm("admin:tenant", pluginHandlers.List))
	mux.Handle("GET /v1/admin/plugins", withPerm("admin:tenant", pluginHandlers.List))
	mux.Handle("GET /admin/plugins/{id}", withPerm("admin:tenant", pluginHandlers.Get))
	mux.Handle("GET /v1/admin/plugins/{id}", withPerm("admin:tenant", pluginHandlers.Get))
	mux.Handle("GET /admin/plugins/{id}/versions", withPerm("admin:tenant", pluginHandlers.ListVersions))
	mux.Handle("GET /v1/admin/plugins/{id}/versions", withPerm("admin:tenant", pluginHandlers.ListVersions))
	mux.Handle("GET /admin/plugins/palette/steps", withPerm("admin:tenant", pluginHandlers.StepPalette))
	mux.Handle("GET /v1/admin/plugins/palette/steps", withPerm("admin:tenant", pluginHandlers.StepPalette))
	mux.Handle("GET /admin/plugins/palette/tools", withPerm("admin:tenant", pluginHandlers.ToolPalette))
	mux.Handle("GET /v1/admin/plugins/palette/tools", withPerm("admin:tenant", pluginHandlers.ToolPalette))
	mux.Handle("GET /admin/plugins/search", withPerm("admin:tenant", pluginHandlers.Search))
	mux.Handle("GET /v1/admin/plugins/search", withPerm("admin:tenant", pluginHandlers.Search))
	mux.Handle("GET /admin/plugins/{id}/schema-changes", withPerm("admin:tenant", pluginHandlers.SchemaChanges))
	mux.Handle("GET /v1/admin/plugins/{id}/schema-changes", withPerm("admin:tenant", pluginHandlers.SchemaChanges))
	mux.Handle("GET /admin/plugins/{id}/invocations", withPerm("admin:tenant", pluginHandlers.Invocations))
	mux.Handle("GET /v1/admin/plugins/{id}/invocations", withPerm("admin:tenant", pluginHandlers.Invocations))
	mux.Handle("POST /admin/plugins/{id}/reload", withPerm("admin:tenant", pluginHandlers.Reload))
	mux.Handle("POST /v1/admin/plugins/{id}/reload", withPerm("admin:tenant", pluginHandlers.Reload))
	mux.Handle("POST /admin/plugins/{id}/disable", withPerm("admin:tenant", pluginHandlers.Disable))
	mux.Handle("POST /v1/admin/plugins/{id}/disable", withPerm("admin:tenant", pluginHandlers.Disable))
	mux.Handle("POST /admin/plugins/{id}/enable", withPerm("admin:tenant", pluginHandlers.Enable))
	mux.Handle("POST /v1/admin/plugins/{id}/enable", withPerm("admin:tenant", pluginHandlers.Enable))
	mux.HandleFunc("GET /vault/signed/{doc_id}", vaultHandlers.SignedDownload)
	mux.Handle("GET /connectors", withAuth(connectorHandlers.List))
	mux.Handle("POST /connectors/{key}/actions/{action}/test", withPerm("workflows:edit", connectorHandlers.TestAction))
	mux.Handle("GET /admin/llm-providers", withPerm("admin:tenant", llmHandlers.ListProviders))
	mux.Handle("GET /v1/admin/llm-providers", withPerm("admin:tenant", llmHandlers.ListProviders))
	mux.Handle("POST /admin/llm-providers", withPerm("admin:tenant", llmHandlers.CreateProvider))
	mux.Handle("POST /v1/admin/llm-providers", withPerm("admin:tenant", llmHandlers.CreateProvider))
	mux.Handle("PUT /admin/llm-providers/{id}", withPerm("admin:tenant", llmHandlers.UpdateProvider))
	mux.Handle("PUT /v1/admin/llm-providers/{id}", withPerm("admin:tenant", llmHandlers.UpdateProvider))
	mux.Handle("DELETE /admin/llm-providers/{id}", withPerm("admin:tenant", llmHandlers.DeleteProvider))
	mux.Handle("DELETE /v1/admin/llm-providers/{id}", withPerm("admin:tenant", llmHandlers.DeleteProvider))
	mux.Handle("POST /admin/llm-providers/{id}/test", withPerm("admin:tenant", llmHandlers.TestProvider))
	mux.Handle("POST /v1/admin/llm-providers/{id}/test", withPerm("admin:tenant", llmHandlers.TestProvider))
	mux.Handle("GET /admin/llm-usage", withPerm("admin:tenant", llmHandlers.UsageSummary))
	mux.Handle("GET /v1/admin/llm-usage", withPerm("admin:tenant", llmHandlers.UsageSummary))
	mux.Handle("GET /admin/llm-usage/details", withPerm("admin:tenant", llmHandlers.UsageDetails))
	mux.Handle("GET /v1/admin/llm-usage/details", withPerm("admin:tenant", llmHandlers.UsageDetails))
	mux.Handle("GET /admin/llm-usage/by-purpose", withPerm("admin:tenant", llmHandlers.UsageByPurpose))
	mux.Handle("GET /v1/admin/llm-usage/by-purpose", withPerm("admin:tenant", llmHandlers.UsageByPurpose))
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

	return chainMiddlewares(mux)
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func shouldStartBackgroundTickers() bool {
	// In integration/unit tests, routers are created many times and DB handles are
	// short-lived. Starting process-lifetime tickers there leaks goroutines and
	// causes noisy "database is closed" churn that can destabilize CI.
	if strings.HasSuffix(os.Args[0], ".test") {
		return false
	}
	return true
}

func chainMiddlewares(next http.Handler) http.Handler {
	h := next
	h = middleware.MaintenanceModeMiddleware(h)
	h = middleware.CorrelationMiddleware(h)
	h = middleware.MetricsMiddleware(h)
	h = middleware.RequestLoggingMiddleware(h)
	return h
}
