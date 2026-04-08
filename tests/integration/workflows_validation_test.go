package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
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

	t.Run("draft save accepts canonical extraction keys", func(t *testing.T) {
		validAST := map[string]any{
			"steps": []map[string]any{
				{
					"id":   "extract_customer_details",
					"type": "extraction",
					"config": map[string]any{
						"document_ref": "case.data.documents.customer_pdf",
						"schema_name":  "customer_onboarding",
						"output_path":  "case.data.extracted.customer",
					},
				},
			},
		}
		raw, _ := json.Marshal(validAST)
		req := httptest.NewRequest(http.MethodPut, "/workflows/"+workflowID.String()+"/versions/draft", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for canonical extraction draft save, got status=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("yaml import accepts rule condition-map outcomes for iterative authoring", func(t *testing.T) {
		yamlBody := `steps:
  - id: review_decision
    type: rule
    depends_on: []
    config:
      outcomes:
        approved:
          condition: "case.data.review.decision == 'approve'"
        rejected:
          condition: "case.data.review.decision == 'reject'"
  - id: insert_customer_onboarding
    type: integration
    depends_on: [review_decision]
    config:
      connector: postgres
      action: insert
`
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "draft.yaml")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write([]byte(yamlBody)); err != nil {
			t.Fatalf("write yaml body: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close multipart writer: %v", err)
		}

		req := httptest.NewRequest(http.MethodPut, "/workflows/"+workflowID.String()+"/yaml/draft", &body)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for yaml import with rule condition-map outcomes, got status=%d body=%s", w.Code, w.Body.String())
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
		var payload struct {
			Errors []struct {
				StepID     string `json:"stepId"`
				Field      string `json:"field"`
				Code       string `json:"code"`
				Message    string `json:"message"`
				Suggestion string `json:"suggestion"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode publish validation payload: %v body=%s", err, w.Body.String())
		}
		if len(payload.Errors) == 0 {
			t.Fatalf("expected at least one publish validation error, got body=%s", w.Body.String())
		}
		gotCodes := make([]string, 0, len(payload.Errors))
		for _, item := range payload.Errors {
			gotCodes = append(gotCodes, item.Code)
		}
		if !slices.Contains(gotCodes, "DANGLING_EDGE") {
			t.Fatalf("expected DANGLING_EDGE code in publish errors, got %v", gotCodes)
		}
	})

	t.Run("publish rejects unknown ai component with suggestion", func(t *testing.T) {
		invalidComponentAST := map[string]any{
			"steps": []map[string]any{
				{
					"id":   "risk",
					"type": "ai_component",
					"config": map[string]any{
						"component":   "does_not_exist_component",
						"output_path": "case.data.ai.risk",
					},
				},
			},
		}
		raw, _ := json.Marshal(invalidComponentAST)
		if _, err := db.ExecContext(ctx, `
UPDATE workflow_versions
SET ast = $2::jsonb
WHERE workflow_id = $1
  AND status = 'draft'
`, workflowID, string(raw)); err != nil {
			t.Fatalf("seed invalid component ast: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/workflows/"+workflowID.String()+"/publish", nil)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid component reference, got status=%d body=%s", w.Code, w.Body.String())
		}

		var payload struct {
			Errors []struct {
				StepID     string `json:"stepId"`
				Field      string `json:"field"`
				Code       string `json:"code"`
				Message    string `json:"message"`
				Suggestion string `json:"suggestion"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode invalid component response: %v body=%s", err, w.Body.String())
		}
		if len(payload.Errors) == 0 {
			t.Fatalf("expected invalid component errors, got %s", w.Body.String())
		}
		first := payload.Errors[0]
		if first.Code != "INVALID_COMPONENT_REF" {
			t.Fatalf("expected INVALID_COMPONENT_REF, got %+v", first)
		}
		if first.StepID != "risk" {
			t.Fatalf("expected stepId risk, got %+v", first)
		}
		if !strings.Contains(first.Suggestion, "document_extraction") {
			t.Fatalf("expected suggested component list, got %+v", first)
		}
	})
}

func TestWorkflowsHTTPIntegration_PublishIdempotentNoDuplicateVersionRows(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	tenantID, adminID, adminEmail := fetchDefaultAdmin(t, ctx, db)
	caseTypeName := "wf_publish_idempotency"
	seedAdditionalCaseType(t, ctx, db, tenantID, adminID, caseTypeName)

	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")
	workflowID := createWorkflowViaAPI(t, router, login.Token, caseTypeName, "wf-idempotent-publish")

	validAST := map[string]any{
		"steps": []map[string]any{
			{
				"id":   "notify",
				"type": "notification",
				"config": map[string]any{
					"channel": "email",
					"message": "ready",
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

	firstPublish := httptest.NewRequest(http.MethodPost, "/workflows/"+workflowID.String()+"/publish", nil)
	firstPublish.Header.Set("Authorization", "Bearer "+login.Token)
	firstW := httptest.NewRecorder()
	router.ServeHTTP(firstW, firstPublish)
	if firstW.Code != http.StatusOK {
		t.Fatalf("expected first publish 200, got status=%d body=%s", firstW.Code, firstW.Body.String())
	}

	secondPublish := httptest.NewRequest(http.MethodPost, "/workflows/"+workflowID.String()+"/publish", nil)
	secondPublish.Header.Set("Authorization", "Bearer "+login.Token)
	secondW := httptest.NewRecorder()
	router.ServeHTTP(secondW, secondPublish)
	if secondW.Code != http.StatusOK {
		t.Fatalf("expected idempotent second publish 200, got status=%d body=%s", secondW.Code, secondW.Body.String())
	}

	var publishedRows int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM workflow_versions
WHERE workflow_id = $1
  AND status = 'published'
`, workflowID).Scan(&publishedRows); err != nil {
		t.Fatalf("count published versions: %v", err)
	}
	if publishedRows != 1 {
		t.Fatalf("expected exactly one published version row after repeated publish, got %d", publishedRows)
	}
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
