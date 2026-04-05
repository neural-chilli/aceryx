package integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neural-chilli/aceryx/api"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
)

func TestCasesHTTPIntegration_CloseAndCancelRejectInvalidJSON(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	tenantID, adminID, adminEmail := fetchDefaultAdmin(t, ctx, db)
	caseID := seedVaultCase(t, ctx, db, tenantID, adminID, "cases_http_validation")

	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")

	t.Run("close rejects malformed json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/cases/"+caseID.String()+"/close", bytes.NewBufferString("{"))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for malformed close payload, got status=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("cancel rejects malformed json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/cases/"+caseID.String()+"/cancel", bytes.NewBufferString("{"))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for malformed cancel payload, got status=%d body=%s", w.Code, w.Body.String())
		}
	})
}
