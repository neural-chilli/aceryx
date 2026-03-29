package integration

import (
	"bytes"
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
	"github.com/neural-chilli/aceryx/internal/tenants"
)

func TestTenantsIntegration_BrandingThemesPreferences(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID := seedTenantWithBranding(t, ctx, db, "tenant-branding-a")
	authz := rbac.NewService(db)
	principalSvc := rbac.NewPrincipalService(db, authz)

	admin, _, err := principalSvc.CreatePrincipal(ctx, tenantID, rbac.CreatePrincipalRequest{
		Type:     "human",
		Name:     "Admin",
		Email:    "admin-tenant-a@example.com",
		Password: "Passw0rd",
		Roles:    []string{"admin"},
	})
	if err != nil {
		t.Fatalf("create admin principal: %v", err)
	}
	viewer, _, err := principalSvc.CreatePrincipal(ctx, tenantID, rbac.CreatePrincipalRequest{
		Type:     "human",
		Name:     "Viewer",
		Email:    "viewer-tenant-a@example.com",
		Password: "Passw0rd",
		Roles:    []string{"viewer"},
	})
	if err != nil {
		t.Fatalf("create viewer principal: %v", err)
	}

	router := api.NewRouterWithServices(db, nil)

	t.Run("unauthenticated branding endpoint resolves by slug and hides non-branding fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/tenant/branding?slug=tenant-branding-a", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}

		payload := map[string]any{}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode branding response: %v", err)
		}
		if payload["company_name"] == nil {
			t.Fatalf("expected company_name in branding payload: %+v", payload)
		}
		if _, ok := payload["terminology"]; ok {
			t.Fatalf("unexpected terminology leak in branding payload: %+v", payload)
		}
		if _, ok := payload["settings"]; ok {
			t.Fatalf("unexpected settings leak in branding payload: %+v", payload)
		}

		reqMissing := httptest.NewRequest(http.MethodGet, "/tenant/branding?slug=missing-tenant", nil)
		wMissing := httptest.NewRecorder()
		router.ServeHTTP(wMissing, reqMissing)
		if wMissing.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing slug, got %d", wMissing.Code)
		}
	})

	adminLogin := loginViaAPI(t, router, tenantID, admin.Email, "Passw0rd")
	viewerLogin := loginViaAPI(t, router, tenantID, viewer.Email, "Passw0rd")

	t.Run("login response includes tenant context and themes", func(t *testing.T) {
		if len(adminLogin.Themes) < 4 {
			t.Fatalf("expected default seeded themes in login response, got %d", len(adminLogin.Themes))
		}
		if len(adminLogin.Tenant.Branding) == 0 {
			t.Fatal("expected branding in login response")
		}
		if len(adminLogin.Tenant.Terminology) == 0 {
			t.Fatal("expected terminology in login response")
		}
	})

	t.Run("admin can update branding and non-admin is denied", func(t *testing.T) {
		body := map[string]any{
			"company_name": "Tenant A Updated",
			"logo_url":     "/vault/tenant-assets/logo-updated.svg",
			"favicon_url":  "/vault/tenant-assets/favicon-updated.ico",
			"colors": map[string]any{
				"primary":   "#0B5FFF",
				"secondary": "#0A2540",
				"accent":    "#06B6D4",
			},
			"powered_by": false,
		}
		raw, _ := json.Marshal(body)

		reqAdmin := httptest.NewRequest(http.MethodPut, "/tenant/branding", bytes.NewReader(raw))
		reqAdmin.Header.Set("Authorization", "Bearer "+adminLogin.Token)
		reqAdmin.Header.Set("Content-Type", "application/json")
		wAdmin := httptest.NewRecorder()
		router.ServeHTTP(wAdmin, reqAdmin)
		if wAdmin.Code != http.StatusOK {
			t.Fatalf("expected admin branding update 200, got %d body=%s", wAdmin.Code, wAdmin.Body.String())
		}

		reqViewer := httptest.NewRequest(http.MethodPut, "/tenant/branding", bytes.NewReader(raw))
		reqViewer.Header.Set("Authorization", "Bearer "+viewerLogin.Token)
		reqViewer.Header.Set("Content-Type", "application/json")
		wViewer := httptest.NewRecorder()
		router.ServeHTTP(wViewer, reqViewer)
		if wViewer.Code != http.StatusForbidden {
			t.Fatalf("expected non-admin 403 for branding update, got %d", wViewer.Code)
		}
	})

	t.Run("theme endpoints enforce permissions and preferences persist with fallback on delete", func(t *testing.T) {
		listReq := httptest.NewRequest(http.MethodGet, "/tenant/themes", nil)
		listReq.Header.Set("Authorization", "Bearer "+viewerLogin.Token)
		listW := httptest.NewRecorder()
		router.ServeHTTP(listW, listReq)
		if listW.Code != http.StatusOK {
			t.Fatalf("expected all users to list themes, got %d", listW.Code)
		}

		createPayload := tenants.CreateThemeRequest{
			Name:      "Corporate Blue",
			Key:       "corporate-blue",
			Mode:      "light",
			Overrides: json.RawMessage(`{"--p-primary-500":"#1E40AF"}`),
			SortOrder: 90,
		}
		createRaw, _ := json.Marshal(createPayload)

		viewerCreateReq := httptest.NewRequest(http.MethodPost, "/tenant/themes", bytes.NewReader(createRaw))
		viewerCreateReq.Header.Set("Authorization", "Bearer "+viewerLogin.Token)
		viewerCreateReq.Header.Set("Content-Type", "application/json")
		viewerCreateW := httptest.NewRecorder()
		router.ServeHTTP(viewerCreateW, viewerCreateReq)
		if viewerCreateW.Code != http.StatusForbidden {
			t.Fatalf("expected non-admin theme create denied, got %d", viewerCreateW.Code)
		}

		adminCreateReq := httptest.NewRequest(http.MethodPost, "/tenant/themes", bytes.NewReader(createRaw))
		adminCreateReq.Header.Set("Authorization", "Bearer "+adminLogin.Token)
		adminCreateReq.Header.Set("Content-Type", "application/json")
		adminCreateW := httptest.NewRecorder()
		router.ServeHTTP(adminCreateW, adminCreateReq)
		if adminCreateW.Code != http.StatusCreated {
			t.Fatalf("expected admin theme create 201, got %d body=%s", adminCreateW.Code, adminCreateW.Body.String())
		}
		var created tenants.Theme
		if err := json.Unmarshal(adminCreateW.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode created theme: %v", err)
		}

		prefRaw, _ := json.Marshal(map[string]any{"theme_id": created.ID})
		prefReq := httptest.NewRequest(http.MethodPut, "/auth/preferences", bytes.NewReader(prefRaw))
		prefReq.Header.Set("Authorization", "Bearer "+viewerLogin.Token)
		prefReq.Header.Set("Content-Type", "application/json")
		prefW := httptest.NewRecorder()
		router.ServeHTTP(prefW, prefReq)
		if prefW.Code != http.StatusOK {
			t.Fatalf("set theme preference failed, status=%d body=%s", prefW.Code, prefW.Body.String())
		}

		logoutReq := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		logoutReq.Header.Set("Authorization", "Bearer "+viewerLogin.Token)
		logoutW := httptest.NewRecorder()
		router.ServeHTTP(logoutW, logoutReq)
		if logoutW.Code != http.StatusOK {
			t.Fatalf("logout failed: %d", logoutW.Code)
		}

		viewerLogin2 := loginViaAPI(t, router, tenantID, viewer.Email, "Passw0rd")
		getPrefReq := httptest.NewRequest(http.MethodGet, "/auth/preferences", nil)
		getPrefReq.Header.Set("Authorization", "Bearer "+viewerLogin2.Token)
		getPrefW := httptest.NewRecorder()
		router.ServeHTTP(getPrefW, getPrefReq)
		if getPrefW.Code != http.StatusOK {
			t.Fatalf("get preferences failed: %d body=%s", getPrefW.Code, getPrefW.Body.String())
		}
		var prefs rbac.UserPreferences
		if err := json.Unmarshal(getPrefW.Body.Bytes(), &prefs); err != nil {
			t.Fatalf("decode preferences: %v", err)
		}
		if prefs.ThemeID == nil || *prefs.ThemeID != created.ID {
			t.Fatalf("expected persisted theme preference %s, got %+v", created.ID, prefs.ThemeID)
		}

		deleteReq := httptest.NewRequest(http.MethodDelete, "/tenant/themes/"+created.ID.String(), nil)
		deleteReq.Header.Set("Authorization", "Bearer "+adminLogin.Token)
		deleteW := httptest.NewRecorder()
		router.ServeHTTP(deleteW, deleteReq)
		if deleteW.Code != http.StatusOK {
			t.Fatalf("delete theme failed: %d body=%s", deleteW.Code, deleteW.Body.String())
		}

		getPrefReq2 := httptest.NewRequest(http.MethodGet, "/auth/preferences", nil)
		getPrefReq2.Header.Set("Authorization", "Bearer "+viewerLogin2.Token)
		getPrefW2 := httptest.NewRecorder()
		router.ServeHTTP(getPrefW2, getPrefReq2)
		if getPrefW2.Code != http.StatusOK {
			t.Fatalf("get preferences post delete failed: %d body=%s", getPrefW2.Code, getPrefW2.Body.String())
		}
		var prefsAfterDelete rbac.UserPreferences
		if err := json.Unmarshal(getPrefW2.Body.Bytes(), &prefsAfterDelete); err != nil {
			t.Fatalf("decode preferences post delete: %v", err)
		}
		if prefsAfterDelete.ThemeID == nil {
			t.Fatal("expected fallback theme_id after deleting selected theme")
		}
		if *prefsAfterDelete.ThemeID == created.ID {
			t.Fatalf("expected fallback away from deleted theme %s", created.ID)
		}
	})

	t.Run("tenant isolation for authenticated theme and branding data", func(t *testing.T) {
		tenantB := seedTenantWithBranding(t, ctx, db, "tenant-branding-b")
		adminB, _, err := principalSvc.CreatePrincipal(ctx, tenantB, rbac.CreatePrincipalRequest{
			Type:     "human",
			Name:     "Admin B",
			Email:    "admin-tenant-b@example.com",
			Password: "Passw0rd",
			Roles:    []string{"admin"},
		})
		if err != nil {
			t.Fatalf("create tenant B admin: %v", err)
		}
		loginB := loginViaAPI(t, router, tenantB, adminB.Email, "Passw0rd")

		createRaw, _ := json.Marshal(tenants.CreateThemeRequest{Name: "Only A", Key: "only-a", Mode: "light", Overrides: json.RawMessage(`{}`), SortOrder: 80})
		adminCreateReq := httptest.NewRequest(http.MethodPost, "/tenant/themes", bytes.NewReader(createRaw))
		adminCreateReq.Header.Set("Authorization", "Bearer "+adminLogin.Token)
		adminCreateReq.Header.Set("Content-Type", "application/json")
		adminCreateW := httptest.NewRecorder()
		router.ServeHTTP(adminCreateW, adminCreateReq)
		if adminCreateW.Code != http.StatusCreated {
			t.Fatalf("create tenant A specific theme failed: %d body=%s", adminCreateW.Code, adminCreateW.Body.String())
		}

		listBReq := httptest.NewRequest(http.MethodGet, "/tenant/themes", nil)
		listBReq.Header.Set("Authorization", "Bearer "+loginB.Token)
		listBW := httptest.NewRecorder()
		router.ServeHTTP(listBW, listBReq)
		if listBW.Code != http.StatusOK {
			t.Fatalf("list tenant B themes failed: %d", listBW.Code)
		}
		var themesB []tenants.Theme
		if err := json.Unmarshal(listBW.Body.Bytes(), &themesB); err != nil {
			t.Fatalf("decode tenant B themes: %v", err)
		}
		for _, th := range themesB {
			if th.Key == "only-a" {
				t.Fatalf("tenant isolation violation: tenant B can see tenant A theme %+v", th)
			}
		}
	})
}

type loginPayload struct {
	Token       string               `json:"token"`
	Tenant      rbac.TenantContext   `json:"tenant"`
	Themes      []rbac.ThemeOption   `json:"themes"`
	Preferences rbac.UserPreferences `json:"preferences"`
}

func loginViaAPI(t *testing.T, router http.Handler, tenantID uuid.UUID, email, password string) loginPayload {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"tenant_id": tenantID, "email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed status=%d body=%s", w.Code, w.Body.String())
	}
	var resp loginPayload
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected login token")
	}
	return resp
}

func TestTenantService_ThemeCRUDAndBrandingRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID := seedTenantWithBranding(t, ctx, db, "tenant-service")
	var principalID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO principals (tenant_id, type, name, email, status)
VALUES ($1, 'human', 'Uploader', 'uploader@example.com', 'active')
RETURNING id
`, tenantID).Scan(&principalID); err != nil {
		t.Fatalf("insert uploader principal: %v", err)
	}

	tenantSvc := tenants.NewTenantService(db)
	themeSvc := tenants.NewThemeService(db)

	terms, err := tenantSvc.UpdateTerminology(ctx, tenantID, tenants.Terminology{"case": "application", "Case": "Application"})
	if err != nil {
		t.Fatalf("update terminology with valid pair: %v", err)
	}
	if terms["case"] != "application" || terms["Case"] != "Application" {
		t.Fatalf("unexpected terminology values: %+v", terms)
	}
	if _, err := tenantSvc.UpdateTerminology(ctx, tenantID, tenants.Terminology{"case": "application"}); err == nil {
		t.Fatal("expected terminology pair validation to fail when capitalized variant missing")
	}

	updatedBranding, err := tenantSvc.UpdateBranding(ctx, tenantID, tenants.Branding{
		CompanyName: "Service Tenant",
		LogoURL:     "/vault/tenant-assets/logo.svg",
		FaviconURL:  "/vault/tenant-assets/favicon.ico",
		Colors:      tenants.BrandingColors{Primary: "#101010", Secondary: "#202020", Accent: "#303030"},
		PoweredBy:   true,
	})
	if err != nil {
		t.Fatalf("update branding: %v", err)
	}
	if updatedBranding.CompanyName != "Service Tenant" || updatedBranding.LogoURL == "" {
		t.Fatalf("unexpected branding update result: %+v", updatedBranding)
	}

	uploadURL, err := tenantSvc.UploadTenantAsset(ctx, tenantID, principalID, "logo.png", "image/png", []byte("png-data"))
	if err != nil {
		t.Fatalf("upload tenant asset: %v", err)
	}
	if uploadURL == "" {
		t.Fatal("expected uploaded tenant asset URL")
	}

	created, err := themeSvc.CreateTheme(ctx, tenantID, tenants.CreateThemeRequest{Name: "Ops", Key: "ops", Mode: "dark", Overrides: json.RawMessage(`{"--p-surface-0":"#111"}`), SortOrder: 100})
	if err != nil {
		t.Fatalf("create theme: %v", err)
	}
	if _, err := themeSvc.CreateTheme(ctx, tenantID, tenants.CreateThemeRequest{Name: "Ops 2", Key: "ops", Mode: "light", Overrides: json.RawMessage(`{}`), SortOrder: 101}); err == nil {
		t.Fatal("expected duplicate tenant theme key to be rejected")
	}

	name := "Ops Updated"
	overrides := json.RawMessage(`{"--p-primary-500":"#4455ee"}`)
	updated, err := themeSvc.UpdateTheme(ctx, tenantID, created.ID, tenants.UpdateThemeRequest{Name: &name, Overrides: &overrides})
	if err != nil {
		t.Fatalf("update theme: %v", err)
	}
	if updated.Name != "Ops Updated" {
		t.Fatalf("expected updated theme name, got %+v", updated)
	}

	listed, err := themeSvc.ListThemes(ctx, tenantID)
	if err != nil {
		t.Fatalf("list themes: %v", err)
	}
	if len(listed) < 5 {
		t.Fatalf("expected 4 default themes + custom, got %d", len(listed))
	}

	if err := themeSvc.DeleteTheme(ctx, tenantID, created.ID); err != nil {
		t.Fatalf("delete theme: %v", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE user_preferences SET updated_at = $3 WHERE principal_id = $1 AND (SELECT tenant_id FROM principals WHERE id = $1) = $2`, principalID, tenantID, time.Now().UTC()); err != nil && err != sql.ErrNoRows {
		t.Fatalf("touch user preferences row for compile-time tenant scoping smoke: %v", err)
	}
}
