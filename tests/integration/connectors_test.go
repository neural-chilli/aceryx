package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api"
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
	"github.com/neural-chilli/aceryx/internal/expressions"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
	"github.com/neural-chilli/aceryx/internal/rbac"
)

func TestConnectorsIntegration_HTTPConnector(t *testing.T) {
	conn := httpconn.New()
	action, ok := findAction(conn.Actions(), "request")
	if !ok {
		t.Fatal("http connector request action not found")
	}

	t.Run("successful request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		res, err := action.Execute(context.Background(), nil, map[string]any{
			"method": "GET",
			"url":    srv.URL,
		})
		if err != nil {
			t.Fatalf("http connector request: %v", err)
		}
		if got, ok := res["status"].(int); !ok || got != 200 {
			t.Fatalf("expected status 200, got %#v", res["status"])
		}
	})

	t.Run("timeout enforced", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		_, err := action.Execute(context.Background(), nil, map[string]any{
			"method":          "GET",
			"url":             srv.URL,
			"timeout_seconds": 1,
		})
		if err == nil {
			t.Fatal("expected timeout error")
		}
	})

	t.Run("non-2xx returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`bad`))
		}))
		defer srv.Close()

		_, err := action.Execute(context.Background(), nil, map[string]any{"method": "GET", "url": srv.URL})
		if err == nil {
			t.Fatal("expected non-2xx error")
		}
	})
}

func TestConnectorsIntegration_WebhookReceiverSignatureAndIdempotency(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "c005-webhook")
	seedAdditionalCaseType(t, ctx, db, tenantID, principalID, "webhook_case")
	_, _ = seedPublishedWorkflow(t, ctx, db, tenantID, principalID, "webhook_case", engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "s1", Type: "human_task"}}})

	if _, err := db.ExecContext(ctx, `INSERT INTO secrets (tenant_id, key, value_encrypted) VALUES ($1, 'webhook_secret', 'secret-1')`, tenantID); err != nil {
		t.Fatalf("insert webhook secret: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO webhook_routes (tenant_id, path, case_type, mode, signature_header, signature_secret_key, idempotency_key_path, created_by)
VALUES ($1, '/inbound/loan', 'webhook_case', 'create', 'X-Signature', 'webhook_secret', 'payload.id', $2)
`, tenantID, principalID); err != nil {
		t.Fatalf("insert webhook route: %v", err)
	}

	handler := webhookreceiver.NewHandler(db, connectors.NewChainedSecretStore(connectors.NewDBSecretStore(db)))

	payload := `{"payload":{"id":"evt-1","company":"Acme"}}`
	mac := hmac.New(sha256.New, []byte("secret-1"))
	mac.Write([]byte(payload))
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/inbound/loan", strings.NewReader(payload))
	req1.SetPathValue("path", "inbound/loan")
	req1.Header.Set("X-Signature", signature)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr1.Code, rr1.Body.String())
	}
	var caseCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases WHERE tenant_id = $1`, tenantID).Scan(&caseCount); err != nil {
		t.Fatalf("count webhook-created cases: %v", err)
	}
	if caseCount != 1 {
		t.Fatalf("expected one webhook-created case, got %d", caseCount)
	}
	var initializedSteps int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE c.tenant_id = $1
`, tenantID).Scan(&initializedSteps); err != nil {
		t.Fatalf("count webhook-initialized case steps: %v", err)
	}
	if initializedSteps == 0 {
		t.Fatal("expected webhook-created case to initialize case_steps")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/inbound/loan", strings.NewReader(payload))
	req2.SetPathValue("path", "inbound/loan")
	req2.Header.Set("X-Signature", signature)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected duplicate status 200, got %d body=%s", rr2.Code, rr2.Body.String())
	}

	var deliveries int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM webhook_deliveries WHERE idempotency_key = 'evt-1'`).Scan(&deliveries); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if deliveries != 1 {
		t.Fatalf("expected one idempotency delivery row, got %d", deliveries)
	}
	var caseCountAfterDuplicate int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases WHERE tenant_id = $1`, tenantID).Scan(&caseCountAfterDuplicate); err != nil {
		t.Fatalf("count cases after duplicate delivery: %v", err)
	}
	if caseCountAfterDuplicate != 1 {
		t.Fatalf("expected duplicate delivery to avoid creating extra cases, got %d", caseCountAfterDuplicate)
	}
}

func TestConnectorsIntegration_EmailSlackTeamsGChatJira(t *testing.T) {
	ctx := context.Background()

	t.Run("webhook sender success and failure", func(t *testing.T) {
		conn := webhooksender.New()
		action, _ := findAction(conn.Actions(), "send")

		successSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`ok`))
		}))
		defer successSrv.Close()
		if _, err := action.Execute(ctx, nil, map[string]any{"url": successSrv.URL, "body": map[string]any{"x": "y"}}); err != nil {
			t.Fatalf("webhook sender success: %v", err)
		}

		failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`failed`))
		}))
		defer failSrv.Close()
		if _, err := action.Execute(ctx, nil, map[string]any{"url": failSrv.URL, "body": map[string]any{"x": "y"}}); err == nil {
			t.Fatal("expected webhook sender failure on non-2xx")
		}
	})

	t.Run("email renders multipart with branding", func(t *testing.T) {
		conn := emailconn.New()
		action, _ := findAction(conn.Actions(), "send")
		out, err := action.Execute(ctx, map[string]string{
			"smtp_host": "mock",
			"smtp_port": "25",
			"from":      "noreply@example.com",
			"username":  "u",
			"password":  "p",
		}, map[string]any{
			"to":            "user@example.com",
			"subject":       "Case update",
			"template":      "task_assigned",
			"company_name":  "Acme Lending",
			"brand_primary": "#123456",
			"case_number":   "LA-000001",
			"body":          "Please review",
		})
		if err != nil {
			t.Fatalf("email send: %v", err)
		}
		mimeBody, _ := out["mime"].(string)
		if !strings.Contains(mimeBody, "multipart/alternative") {
			t.Fatalf("expected multipart mime, got %s", mimeBody)
		}
		if !strings.Contains(mimeBody, "Acme Lending") {
			t.Fatalf("expected branding in email output, got %s", mimeBody)
		}
	})

	t.Run("slack send message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ok":true,"ts":"1.2"}`))
		}))
		defer srv.Close()
		conn := slackconn.New()
		action, _ := findAction(conn.Actions(), "send_message")
		_, err := action.Execute(ctx, map[string]string{"bot_token": "x", "api_base_url": srv.URL}, map[string]any{"channel": "#ops", "text": "hello"})
		if err != nil {
			t.Fatalf("slack send: %v", err)
		}
	})

	t.Run("teams send message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`ok`))
		}))
		defer srv.Close()
		conn := teamsconn.New()
		action, _ := findAction(conn.Actions(), "send_message")
		_, err := action.Execute(ctx, map[string]string{"webhook_url": srv.URL}, map[string]any{"title": "Update", "text": "Case changed"})
		if err != nil {
			t.Fatalf("teams send: %v", err)
		}
	})

	t.Run("google chat send message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`ok`))
		}))
		defer srv.Close()
		conn := gchatconn.New()
		action, _ := findAction(conn.Actions(), "send_message")
		_, err := action.Execute(ctx, map[string]string{"webhook_url": srv.URL}, map[string]any{"text": "Case changed"})
		if err != nil {
			t.Fatalf("google chat send: %v", err)
		}
	})

	t.Run("jira create issue", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"10001","key":"OPS-1"}`))
		}))
		defer srv.Close()
		conn := jiraconn.New()
		action, _ := findAction(conn.Actions(), "create_issue")
		_, err := action.Execute(ctx, map[string]string{"base_url": srv.URL, "email": "u@example.com", "api_token": "tok"}, map[string]any{
			"project":     "OPS",
			"issue_type":  "Task",
			"summary":     "Review case",
			"description": "details",
		})
		if err != nil {
			t.Fatalf("jira create issue: %v", err)
		}
	})
}

func TestConnectorsIntegration_DocGenAndIntegrationExecutor(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "c005-docgen")
	caseTypeID := seedAdditionalCaseType(t, ctx, db, tenantID, principalID, "doc_case")
	workflowID, _ := seedPublishedWorkflow(t, ctx, db, tenantID, principalID, "doc_case", engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "doc", Type: "integration"}}})
	var caseID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, 'DOC-000001', 'open', '{"applicant":{"company_name":"Acme"},"loan":{"amount":50000}}'::jsonb, $3, $4, 1)
RETURNING id
`, tenantID, caseTypeID, principalID, workflowID).Scan(&caseID); err != nil {
		t.Fatalf("insert doc case: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO case_steps (case_id, step_id, state, events, metadata) VALUES ($1, 'doc', 'active', '[]'::jsonb, '{}'::jsonb)`, caseID); err != nil {
		t.Fatalf("insert step: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO document_templates (tenant_id, name, version, status, template, created_by)
VALUES ($1, 'offer_letter_v2', 1, 'active', $2::jsonb, $3)
`, tenantID, `{"filename":"Offer-{{case.case_number}}.pdf","layout":[{"type":"header","company":"{{tenant.branding.company_name}}","date":"{{now}}"},{"type":"paragraph","text":"Dear {{case.data.applicant.company_name}}"},{"type":"table","rows":[["Amount","{{case.data.loan.amount | formatCurrency}}"]]}]}`, principalID); err != nil {
		t.Fatalf("insert document template: %v", err)
	}

	docAction, _ := findAction(docgenconn.New(db, nil).Actions(), "generate_pdf")
	out, err := docAction.Execute(ctx, nil, map[string]any{
		"template":   "offer_letter_v2",
		"_case_id":   caseID.String(),
		"_step_id":   "doc",
		"_tenant_id": tenantID.String(),
		"_actor_id":  principalID.String(),
		"case": map[string]any{
			"case_number": "DOC-000001",
			"data": map[string]any{
				"applicant": map[string]any{"company_name": "Acme"},
				"loan":      map[string]any{"amount": 50000},
			},
		},
		"tenant": map[string]any{"branding": map[string]any{"company_name": "Acme"}},
	})
	if err != nil {
		t.Fatalf("docgen generate_pdf: %v", err)
	}
	if !strings.HasSuffix(out["filename"].(string), ".pdf") {
		t.Fatalf("expected pdf filename, got %v", out["filename"])
	}
	var storedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE case_id = $1 AND step_id = 'doc'`, caseID).Scan(&storedCount); err != nil {
		t.Fatalf("count vault docs: %v", err)
	}
	if storedCount != 1 {
		t.Fatalf("expected one stored vault document row, got %d", storedCount)
	}

	// Executor end-to-end with runtime secret resolution + engine integration step.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":"pong"}`))
	}))
	defer server.Close()

	if _, err := db.ExecContext(ctx, `INSERT INTO secrets (tenant_id, key, value_encrypted) VALUES ($1, 'api_token', 'secret-token') ON CONFLICT (tenant_id, key) DO UPDATE SET value_encrypted = EXCLUDED.value_encrypted`, tenantID); err != nil {
		t.Fatalf("insert api secret: %v", err)
	}

	reg := connectors.NewRegistry()
	reg.Register(httpconn.New())
	exec := connectors.NewExecutor(db, reg, connectors.NewChainedSecretStore(connectors.NewDBSecretStore(db)))
	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	en.RegisterExecutor("integration", exec)

	cfg, _ := json.Marshal(map[string]any{
		"connector": "http",
		"action":    "request",
		"input": map[string]any{
			"method": "GET",
			"url":    server.URL,
			"headers": map[string]any{
				"Authorization": "Bearer {{secrets.api_token}}",
			},
			"timeout_seconds": 2,
		},
	})
	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "int1", Type: "integration", Config: cfg}}}
	caseID2 := seedEngineCase(t, ctx, db, ast)
	var caseTenantID uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1`, caseID2).Scan(&caseTenantID); err != nil {
		t.Fatalf("load tenant for integration case: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO secrets (tenant_id, key, value_encrypted) VALUES ($1, 'api_token', 'secret-token') ON CONFLICT (tenant_id, key) DO UPDATE SET value_encrypted = EXCLUDED.value_encrypted`, caseTenantID); err != nil {
		t.Fatalf("insert api secret for integration case tenant: %v", err)
	}

	if err := en.EvaluateDAG(ctx, caseID2); err != nil {
		t.Fatalf("evaluate dag for integration executor: %v", err)
	}
	waitForStepState(t, ctx, db, caseID2, "int1", engine.StateCompleted)
}

func TestConnectorsIntegration_APIListAndTestAction(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	authSvc := rbac.NewAuthService(db, "test-secret", time.Hour)
	var tenantID uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT id FROM tenants WHERE slug = 'default'`).Scan(&tenantID); err != nil {
		t.Fatalf("load default tenant id: %v", err)
	}
	login, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantID, Email: "admin@localhost", Password: "admin"})
	if err != nil {
		t.Fatalf("admin login: %v", err)
	}

	router := api.NewRouterWithServices(db, nil)

	listReq := httptest.NewRequest(http.MethodGet, "/connectors", nil)
	listReq.Header.Set("Authorization", "Bearer "+login.Token)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /connectors expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), `"key":"http"`) {
		t.Fatalf("expected http connector in list response: %s", listRec.Body.String())
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	body := fmt.Sprintf(`{"input":{"method":"GET","url":"%s"}}`, srv.URL)
	testReq := httptest.NewRequest(http.MethodPost, "/connectors/http/actions/request/test", strings.NewReader(body))
	testReq.Header.Set("Authorization", "Bearer "+login.Token)
	testReq.Header.Set("Content-Type", "application/json")
	testRec := httptest.NewRecorder()
	router.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("POST /connectors/http/actions/request/test expected 200, got %d body=%s", testRec.Code, testRec.Body.String())
	}
}

func findAction(actions []connectors.ActionSpec, key string) (connectors.ActionSpec, bool) {
	for _, action := range actions {
		if action.Key == key {
			return action, true
		}
	}
	return connectors.ActionSpec{}, false
}
