package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
)

func TestWorkflowsHTTPIntegration_RejectInvalidASTOnDraftSaveAndPublish(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	tenantID, adminID, adminEmail := fetchDefaultAdmin(t, ctx, db)
	caseTypeName := "wf_validation_case_type"
	seedAdditionalCaseType(t, ctx, db, tenantID, adminID, caseTypeName)

	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")
	workflowID := createWorkflowViaAPI(t, router, login.Token, caseTypeName, "wf-validation")

	t.Run("draft save rejects malformed integration config", func(t *testing.T) {
		invalidAST := map[string]any{
			"steps": []map[string]any{
				{
					"id":   "call_api",
					"type": "integration",
					"config": map[string]any{
						"connector": "http",
					},
				},
			},
		}
		raw, _ := json.Marshal(invalidAST)
		req := httptest.NewRequest(http.MethodPut, "/workflows/"+workflowID.String()+"/versions/draft", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid draft AST, got status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "invalid workflow ast:") {
			t.Fatalf("expected invalid workflow ast error, got body=%s", w.Body.String())
		}
	})

	t.Run("publish rejects invalid draft ast persisted in db", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `
UPDATE workflow_versions
SET ast = $2::jsonb
WHERE workflow_id = $1
  AND status = 'draft'
`, workflowID, `{"steps":[{"id":"route","type":"rule","outcomes":{"approve":["missing"]},"config":{"outcomes":[{"name":"approve","condition":"true","target":"missing"}]}}]}`); err != nil {
			t.Fatalf("seed invalid draft ast: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/workflows/"+workflowID.String()+"/publish", nil)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for publish invalid ast, got status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "invalid workflow ast:") {
			t.Fatalf("expected invalid workflow ast error, got body=%s", w.Body.String())
		}
	})
}

func TestWorkflowsHTTPIntegration_ExportYAMLVersion(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	tenantID, adminID, adminEmail := fetchDefaultAdmin(t, ctx, db)
	caseTypeName := "wf_export_case_type"
	seedAdditionalCaseType(t, ctx, db, tenantID, adminID, caseTypeName)

	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")
	workflowID := createWorkflowViaAPI(t, router, login.Token, caseTypeName, "wf-export-version")

	validAST := map[string]any{
		"steps": []map[string]any{
			{
				"id":   "call_api",
				"type": "integration",
				"config": map[string]any{
					"connector": "http",
					"action":    "request",
				},
			},
		},
	}
	raw, _ := json.Marshal(validAST)
	putReq := httptest.NewRequest(http.MethodPut, "/workflows/"+workflowID.String()+"/versions/draft", bytes.NewReader(raw))
	putReq.Header.Set("Authorization", "Bearer "+login.Token)
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	router.ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("expected draft save 200, got status=%d body=%s", putW.Code, putW.Body.String())
	}

	t.Run("exports specific version yaml", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/workflows/"+workflowID.String()+"/yaml/1", nil)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for versioned yaml export, got status=%d body=%s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "call_api") || !strings.Contains(body, "integration") {
			t.Fatalf("expected exported yaml to include draft ast steps, got body=%s", body)
		}
	})

	t.Run("returns 404 for unknown version", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/workflows/"+workflowID.String()+"/yaml/999", nil)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unknown yaml version, got status=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("returns 400 for invalid version token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/workflows/"+workflowID.String()+"/yaml/not-a-number", nil)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid yaml version token, got status=%d body=%s", w.Code, w.Body.String())
		}
	})
}

func createWorkflowViaAPI(t *testing.T, router http.Handler, bearerToken, caseTypeID, name string) uuid.UUID {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":         name,
		"case_type_id": caseTypeID,
	})
	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create workflow failed status=%d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create workflow response: %v", err)
	}
	idRaw, _ := payload["id"].(string)
	if strings.TrimSpace(idRaw) == "" {
		t.Fatalf("create workflow response missing id: %v", payload)
	}
	workflowID, err := uuid.Parse(idRaw)
	if err != nil {
		t.Fatalf("parse workflow id %q: %v", idRaw, err)
	}
	return workflowID
}
