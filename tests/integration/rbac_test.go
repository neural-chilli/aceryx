package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api"
	"github.com/neural-chilli/aceryx/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

func TestRBACIntegration_PrincipalAndAuthFlows(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID := seedTenantWithBranding(t, ctx, db, "rbac-a")
	authz := rbac.NewService(db)
	principalSvc := rbac.NewPrincipalService(db, authz)
	roleSvc := rbac.NewRoleService(db, authz)
	authSvc := rbac.NewAuthService(db, "test-secret", time.Hour)

	roles, err := roleSvc.ListRoles(ctx, tenantID)
	if err != nil {
		t.Fatalf("list default roles: %v", err)
	}
	if len(roles) < 4 {
		t.Fatalf("expected default roles to be seeded, got %d", len(roles))
	}

	human, _, err := principalSvc.CreatePrincipal(ctx, tenantID, rbac.CreatePrincipalRequest{
		Type:     "human",
		Name:     "Jane",
		Email:    "jane@example.com",
		Password: "Passw0rd",
		Roles:    []string{"admin"},
	})
	if err != nil {
		t.Fatalf("create human principal: %v", err)
	}

	var passwordHash string
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM principals WHERE id = $1 AND tenant_id = $2`, human.ID, tenantID).Scan(&passwordHash); err != nil {
		t.Fatalf("load principal password hash: %v", err)
	}
	if passwordHash == "Passw0rd" {
		t.Fatal("password hash should not store plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("Passw0rd")); err != nil {
		t.Fatalf("stored password hash does not match password: %v", err)
	}

	agent, apiKey, err := principalSvc.CreatePrincipal(ctx, tenantID, rbac.CreatePrincipalRequest{
		Type:  "agent",
		Name:  "Risk Agent",
		Roles: []string{"viewer"},
	})
	if err != nil {
		t.Fatalf("create agent principal: %v", err)
	}
	if apiKey == "" {
		t.Fatal("expected one-time api key for agent principal")
	}
	var apiKeyHash sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT api_key_hash FROM principals WHERE id = $1`, agent.ID).Scan(&apiKeyHash); err != nil {
		t.Fatalf("load agent api key hash: %v", err)
	}
	if !apiKeyHash.Valid || apiKeyHash.String == "" {
		t.Fatal("expected agent api key hash to be stored")
	}

	loginResp, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantID, Email: "jane@example.com", Password: "Passw0rd", IPAddress: "127.0.0.1", UserAgent: "go-test"})
	if err != nil {
		t.Fatalf("valid login: %v", err)
	}
	if loginResp.Token == "" || loginResp.Principal.ID != human.ID {
		t.Fatal("expected token and principal in login response")
	}
	if string(loginResp.Tenant.Branding) == "" || string(loginResp.Tenant.Terminology) == "" {
		t.Fatal("expected tenant branding and terminology in login response")
	}

	if _, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantID, Email: "unknown@example.com", Password: "Passw0rd"}); err == nil {
		t.Fatal("expected invalid email login to fail")
	}
	if _, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantID, Email: "jane@example.com", Password: "wrongpass1"}); err == nil {
		t.Fatal("expected wrong password login to fail")
	}

	ap, err := authSvc.AuthenticateBearer(ctx, loginResp.Token)
	if err != nil {
		t.Fatalf("authenticate bearer jwt: %v", err)
	}
	if ap.ID != human.ID || ap.TenantID != tenantID {
		t.Fatalf("unexpected auth principal from token: %+v", ap)
	}

	if err := authSvc.Logout(ctx, tenantID, human.ID, *ap.SessionID); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := authSvc.AuthenticateBearer(ctx, loginResp.Token); err == nil {
		t.Fatal("expected deleted session token to be invalid")
	}

	loginResp2, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantID, Email: "jane@example.com", Password: "Passw0rd"})
	if err != nil {
		t.Fatalf("login for password change: %v", err)
	}
	ap2, err := authSvc.AuthenticateBearer(ctx, loginResp2.Token)
	if err != nil {
		t.Fatalf("authenticate bearer for password change: %v", err)
	}

	if err := authSvc.ChangePassword(ctx, tenantID, human.ID, *ap2.SessionID, rbac.ChangePasswordRequest{CurrentPassword: "wrong", NewPassword: "Newpass1"}); err == nil {
		t.Fatal("expected password change with wrong current password to fail")
	}
	if err := authSvc.ChangePassword(ctx, tenantID, human.ID, *ap2.SessionID, rbac.ChangePasswordRequest{CurrentPassword: "Passw0rd", NewPassword: "Newpass1"}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	var changedCount int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_events
WHERE tenant_id = $1
  AND principal_id = $2
  AND event_type = 'password_changed'
  AND data::text NOT ILIKE '%password%'
`, tenantID, human.ID).Scan(&changedCount); err != nil {
		t.Fatalf("verify password_changed auth event: %v", err)
	}
	if changedCount == 0 {
		t.Fatal("expected password_changed auth event without password data")
	}

	if _, err := authSvc.AuthenticateBearer(ctx, apiKey); err != nil {
		t.Fatalf("authenticate api key: %v", err)
	}

	if err := principalSvc.DisablePrincipal(ctx, tenantID, human.ID); err != nil {
		t.Fatalf("disable principal: %v", err)
	}
	if _, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantID, Email: "jane@example.com", Password: "Newpass1"}); err == nil {
		t.Fatal("expected disabled principal login to fail")
	}
}

func TestRBACIntegration_RouteMiddlewareAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantA := seedTenantWithBranding(t, ctx, db, "rbac-tenant-a")
	tenantB := seedTenantWithBranding(t, ctx, db, "rbac-tenant-b")

	authz := rbac.NewService(db)
	principalSvc := rbac.NewPrincipalService(db, authz)
	authSvc := rbac.NewAuthService(db, "test-secret", time.Hour)

	adminA, _, err := principalSvc.CreatePrincipal(ctx, tenantA, rbac.CreatePrincipalRequest{Type: "human", Name: "Admin A", Email: "admin-a@example.com", Password: "Passw0rd", Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("create admin A: %v", err)
	}
	viewerA, _, err := principalSvc.CreatePrincipal(ctx, tenantA, rbac.CreatePrincipalRequest{Type: "human", Name: "Viewer A", Email: "viewer-a@example.com", Password: "Passw0rd", Roles: []string{"viewer"}})
	if err != nil {
		t.Fatalf("create viewer A: %v", err)
	}
	adminB, _, err := principalSvc.CreatePrincipal(ctx, tenantB, rbac.CreatePrincipalRequest{Type: "human", Name: "Admin B", Email: "admin-b@example.com", Password: "Passw0rd", Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("create admin B: %v", err)
	}

	caseTypeB, workflowB := seedMinimalCaseTypeAndWorkflow(t, ctx, db, tenantB, adminB.ID)
	caseB := seedCaseForTenant(t, ctx, db, tenantB, adminB.ID, caseTypeB, workflowB, "TENANTB-000001")
	if _, err := db.ExecContext(ctx, `INSERT INTO case_steps (case_id, step_id, state) VALUES ($1, 's1', 'pending')`, caseB); err != nil {
		t.Fatalf("insert case step tenant B: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO vault_documents (tenant_id, case_id, filename, mime_type, size_bytes, content_hash, storage_uri, uploaded_by)
VALUES ($1, $2, 'doc.pdf', 'application/pdf', 10, 'h1', 's3://doc', $3)
`, tenantB, caseB, adminB); err != nil {
		t.Fatalf("insert vault document tenant B: %v", err)
	}

	router := api.NewRouterWithServices(db, nil)

	req401 := httptest.NewRequest(http.MethodGet, "/cases", nil)
	w401 := httptest.NewRecorder()
	router.ServeHTTP(w401, req401)
	if w401.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth header, got %d", w401.Code)
	}

	loginViewer, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantA, Email: viewerA.Email, Password: "Passw0rd"})
	if err != nil {
		t.Fatalf("login viewer A: %v", err)
	}

	req403 := httptest.NewRequest(http.MethodGet, "/admin/roles", nil)
	req403.Header.Set("Authorization", "Bearer "+loginViewer.Token)
	w403 := httptest.NewRecorder()
	router.ServeHTTP(w403, req403)
	if w403.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthorized permission, got %d", w403.Code)
	}

	var deniedCount int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM auth_events
WHERE tenant_id = $1 AND principal_id = $2 AND event_type = 'permission_denied' AND permission = 'admin:roles' AND resource_path = '/admin/roles'
`, tenantA, viewerA.ID).Scan(&deniedCount); err != nil {
		t.Fatalf("query denied audit events: %v", err)
	}
	if deniedCount == 0 {
		t.Fatal("expected denied access audit event")
	}

	loginAdminA, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantA, Email: adminA.Email, Password: "Passw0rd"})
	if err != nil {
		t.Fatalf("login admin A: %v", err)
	}

	reqCase := httptest.NewRequest(http.MethodGet, "/cases/"+caseB.String(), nil)
	reqCase.Header.Set("Authorization", "Bearer "+loginAdminA.Token)
	wCase := httptest.NewRecorder()
	router.ServeHTTP(wCase, reqCase)
	if wCase.Code != http.StatusNotFound {
		t.Fatalf("expected tenant A to not access tenant B case, got %d", wCase.Code)
	}

	reqPrincipals := httptest.NewRequest(http.MethodGet, "/admin/principals", nil)
	reqPrincipals.Header.Set("Authorization", "Bearer "+loginAdminA.Token)
	wPrincipals := httptest.NewRecorder()
	router.ServeHTTP(wPrincipals, reqPrincipals)
	if wPrincipals.Code != http.StatusOK {
		t.Fatalf("list principals for tenant A failed, status=%d body=%s", wPrincipals.Code, wPrincipals.Body.String())
	}
	var principals []rbac.Principal
	if err := json.Unmarshal(wPrincipals.Body.Bytes(), &principals); err != nil {
		t.Fatalf("decode principals response: %v", err)
	}
	for _, p := range principals {
		if p.TenantID != tenantA {
			t.Fatalf("tenant isolation violation in principals list: got tenant %s expected %s", p.TenantID, tenantA)
		}
	}

	caseTypeA, workflowA := seedMinimalCaseTypeAndWorkflow(t, ctx, db, tenantA, adminA.ID)
	var conflictCaseCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases WHERE tenant_id = $1 AND case_type_id = $2`, tenantA, caseTypeA).Scan(&conflictCaseCount); err != nil {
		t.Fatalf("count tenant A cases: %v", err)
	}
	if conflictCaseCount != 0 {
		t.Fatalf("expected no pre-existing tenant A cases before explicit create, got %d", conflictCaseCount)
	}
	_ = workflowA
}

func TestRBACIntegration_RolePermissionInvalidationAndSessionCleanup(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID := seedTenantWithBranding(t, ctx, db, "rbac-role")
	authz := rbac.NewService(db)
	principalSvc := rbac.NewPrincipalService(db, authz)
	roleSvc := rbac.NewRoleService(db, authz)
	authSvc := rbac.NewAuthService(db, "test-secret", time.Hour)

	worker, _, err := principalSvc.CreatePrincipal(ctx, tenantID, rbac.CreatePrincipalRequest{Type: "human", Name: "Worker", Email: "worker@example.com", Password: "Passw0rd", Roles: []string{"viewer"}})
	if err != nil {
		t.Fatalf("create worker principal: %v", err)
	}
	if err := authz.Authorize(ctx, worker.ID, "cases:update"); err == nil {
		t.Fatal("expected cases:update to be denied before role update")
	}

	roles, err := roleSvc.ListRoles(ctx, tenantID)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	var viewerRoleID uuid.UUID
	for _, role := range roles {
		if role.Name == "viewer" {
			viewerRoleID = role.ID
			break
		}
	}
	if viewerRoleID == uuid.Nil {
		t.Fatal("viewer role not found")
	}
	if _, err := roleSvc.UpdateRolePermissions(ctx, tenantID, viewerRoleID, []string{"cases:read", "cases:update"}); err != nil {
		t.Fatalf("update viewer role permissions: %v", err)
	}
	if err := authz.Authorize(ctx, worker.ID, "cases:update"); err != nil {
		t.Fatalf("expected role permission update to invalidate cache and allow cases:update: %v", err)
	}

	login, err := authSvc.Login(ctx, rbac.LoginRequest{TenantID: &tenantID, Email: worker.Email, Password: "Passw0rd"})
	if err != nil {
		t.Fatalf("login worker: %v", err)
	}
	if _, err := authSvc.AuthenticateBearer(ctx, login.Token); err != nil {
		t.Fatalf("authenticate worker token pre-cleanup: %v", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE sessions SET expires_at = now() - interval '10 seconds'`); err != nil {
		t.Fatalf("expire sessions: %v", err)
	}
	authSvc.SetCleanupInterval(20 * time.Millisecond)
	cleanupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go authSvc.StartSessionCleanup(cleanupCtx)
	time.Sleep(80 * time.Millisecond)

	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE expires_at < now()`).Scan(&remaining); err != nil {
		t.Fatalf("count expired sessions after cleanup ticker: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected session cleanup ticker to remove expired sessions, remaining=%d", remaining)
	}
}

func seedTenantWithBranding(t *testing.T, ctx context.Context, db *sql.DB, slug string) uuid.UUID {
	t.Helper()
	var tenantID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO tenants (name, slug, branding, terminology, settings)
VALUES ($1, $2, '{"company_name":"Aceryx Test"}'::jsonb, '{"case":"Case"}'::jsonb, '{}'::jsonb)
RETURNING id
`, "Tenant "+slug, slug).Scan(&tenantID)
	if err != nil {
		t.Fatalf("insert tenant with branding: %v", err)
	}
	return tenantID
}

func seedMinimalCaseTypeAndWorkflow(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	caseTypeName := "ct-" + uuid.NewString()[:8]
	workflowName := "wf-" + uuid.NewString()[:8]
	var caseTypeID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO case_types (tenant_id, name, version, schema, status, created_by)
VALUES ($1, $2, 1, '{}'::jsonb, 'active', $3)
RETURNING id
`, tenantID, caseTypeName, principalID).Scan(&caseTypeID); err != nil {
		t.Fatalf("insert case type: %v", err)
	}
	var workflowID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO workflows (tenant_id, name, case_type, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id
`, tenantID, workflowName, caseTypeName, principalID).Scan(&workflowID); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workflow_versions (workflow_id, version, status, ast, yaml_source, created_by, published_at)
VALUES ($1, 1, 'published', '{"steps":[{"id":"s1","type":"human_task"}]}'::jsonb, '', $2, now())
`, workflowID, principalID); err != nil {
		t.Fatalf("insert workflow version: %v", err)
	}
	return caseTypeID, workflowID
}

func seedCaseForTenant(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID, caseTypeID, workflowID uuid.UUID, caseNumber string) uuid.UUID {
	t.Helper()
	var caseID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'open', '{}'::jsonb, $4, $5, 1)
RETURNING id
`, tenantID, caseTypeID, caseNumber, principalID, workflowID).Scan(&caseID)
	if err != nil {
		t.Fatalf("insert case: %v", err)
	}
	return caseID
}
