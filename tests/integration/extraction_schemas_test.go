package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
)

func TestExtractionSchemasHTTPIntegration_CRUD(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	tenantID, _, adminEmail := fetchDefaultAdmin(t, ctx, db)
	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")

	fields := []map[string]any{
		{"name": "company_number", "type": "string", "required": true},
		{"name": "loan_amount", "type": "number", "required": true},
	}
	createBody, _ := json.Marshal(map[string]any{
		"name":        "Loan Application",
		"description": "Extract borrower details from uploaded application documents.",
		"fields":      fields,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/extraction-schemas", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+login.Token)
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("expected 201 create extraction schema, got status=%d body=%s", createW.Code, createW.Body.String())
	}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatalf("expected created extraction schema id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/extraction-schemas", nil)
	listReq.Header.Set("Authorization", "Bearer "+login.Token)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected 200 list extraction schemas, got status=%d body=%s", listW.Code, listW.Body.String())
	}
	var listed struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Items) == 0 {
		t.Fatalf("expected extraction schema in list")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/extraction-schemas/"+created.ID.String(), nil)
	getReq.Header.Set("Authorization", "Bearer "+login.Token)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("expected 200 get extraction schema, got status=%d body=%s", getW.Code, getW.Body.String())
	}

	updateBody, _ := json.Marshal(map[string]any{
		"name":        "Loan Application v2",
		"description": "Updated extraction schema",
		"fields":      fields,
	})
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/extraction-schemas/"+created.ID.String(), bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer "+login.Token)
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	router.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("expected 200 update extraction schema, got status=%d body=%s", updateW.Code, updateW.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/extraction-schemas/"+created.ID.String(), nil)
	deleteReq.Header.Set("Authorization", "Bearer "+login.Token)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("expected 200 delete extraction schema, got status=%d body=%s", deleteW.Code, deleteW.Body.String())
	}

	getAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/api/v1/extraction-schemas/"+created.ID.String(), nil)
	getAfterDeleteReq.Header.Set("Authorization", "Bearer "+login.Token)
	getAfterDeleteW := httptest.NewRecorder()
	router.ServeHTTP(getAfterDeleteW, getAfterDeleteReq)
	if getAfterDeleteW.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after deletion, got status=%d body=%s", getAfterDeleteW.Code, getAfterDeleteW.Body.String())
	}
}

func TestExtractionSchemasHTTPIntegration_Validation(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	tenantID, _, adminEmail := fetchDefaultAdmin(t, ctx, db)
	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")

	t.Run("rejects non-array fields", func(t *testing.T) {
		raw := []byte(`{"name":"Bad","fields":{"field":"value"}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/extraction-schemas", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for non-array fields, got status=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects missing name", func(t *testing.T) {
		raw := []byte(`{"name":"","fields":[]}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/extraction-schemas", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing name, got status=%d body=%s", w.Code, w.Body.String())
		}
	})
}
