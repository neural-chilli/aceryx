package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
)

func TestExtractionJobsHTTPIntegration_ActionsAndQueries(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	tenantID, adminID, adminEmail := fetchDefaultAdmin(t, ctx, db)
	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")

	caseID := seedVaultCase(t, ctx, db, tenantID, adminID, "extract_case_"+uuid.NewString()[:8])
	schemaID := seedExtractionSchema(t, ctx, db, tenantID)
	documentID := seedExtractionDocument(t, ctx, db, tenantID, caseID, adminID)
	jobID := seedExtractionJob(t, ctx, db, tenantID, caseID, documentID, schemaID)
	fieldID := seedExtractionField(t, ctx, db, jobID)

	t.Run("get job", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/extraction-jobs/"+jobID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 get job, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("list fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/extraction-jobs/"+jobID.String()+"/fields", nil)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 list fields, got %d body=%s", w.Code, w.Body.String())
		}
		var payload struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode list fields response: %v", err)
		}
		if len(payload.Items) == 0 {
			t.Fatalf("expected at least one extraction field")
		}
	})

	t.Run("confirm field", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/extraction-fields/"+fieldID.String()+"/confirm", nil)
		req.Header.Set("Authorization", "Bearer "+login.Token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 confirm field, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("correct field and list corrections", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"corrected_value": "12345678"})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/extraction-fields/"+fieldID.String()+"/correct", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 correct field, got %d body=%s", w.Code, w.Body.String())
		}

		since := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
		listReq := httptest.NewRequest(http.MethodGet, "/api/v1/extraction-corrections?schema_id="+schemaID.String()+"&since="+since, nil)
		listReq.Header.Set("Authorization", "Bearer "+login.Token)
		listW := httptest.NewRecorder()
		router.ServeHTTP(listW, listReq)
		if listW.Code != http.StatusOK {
			t.Fatalf("expected 200 list corrections, got %d body=%s", listW.Code, listW.Body.String())
		}
		var payload struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(listW.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode list corrections response: %v", err)
		}
		if len(payload.Items) == 0 {
			t.Fatalf("expected at least one correction after field correction")
		}
	})

	t.Run("accept and reject job", func(t *testing.T) {
		acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/extraction-jobs/"+jobID.String()+"/accept", nil)
		acceptReq.Header.Set("Authorization", "Bearer "+login.Token)
		acceptW := httptest.NewRecorder()
		router.ServeHTTP(acceptW, acceptReq)
		if acceptW.Code != http.StatusOK {
			t.Fatalf("expected 200 accept job, got %d body=%s", acceptW.Code, acceptW.Body.String())
		}

		rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/extraction-jobs/"+jobID.String()+"/reject", nil)
		rejectReq.Header.Set("Authorization", "Bearer "+login.Token)
		rejectW := httptest.NewRecorder()
		router.ServeHTTP(rejectW, rejectReq)
		if rejectW.Code != http.StatusOK {
			t.Fatalf("expected 200 reject job, got %d body=%s", rejectW.Code, rejectW.Body.String())
		}
	})
}

func seedExtractionSchema(t *testing.T, ctx context.Context, db *sql.DB, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	fields := `[ {"name":"company_number","type":"string","required":true} ]`
	var schemaID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO extraction_schemas (tenant_id, name, description, fields)
VALUES ($1, $2, $3, $4::jsonb)
RETURNING id
`, tenantID, "Loan Extraction", "schema", fields).Scan(&schemaID); err != nil {
		t.Fatalf("insert extraction schema: %v", err)
	}
	return schemaID
}

func seedExtractionDocument(t *testing.T, ctx context.Context, db *sql.DB, tenantID, caseID, adminID uuid.UUID) uuid.UUID {
	t.Helper()
	var documentID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO vault_documents (
    tenant_id,
    case_id,
    step_id,
    filename,
    mime_type,
    size_bytes,
    content_hash,
    storage_uri,
    uploaded_by,
    metadata
) VALUES ($1, $2, 'extract', 'application.pdf', 'application/pdf', 1234, $3, $4, $5, '{}'::jsonb)
RETURNING id
`, tenantID, caseID, fmt.Sprintf("hash-%s", uuid.NewString()[:8]), fmt.Sprintf("local://%s", uuid.NewString()[:8]), adminID).Scan(&documentID); err != nil {
		t.Fatalf("insert extraction document: %v", err)
	}
	return documentID
}

func seedExtractionJob(t *testing.T, ctx context.Context, db *sql.DB, tenantID, caseID, documentID, schemaID uuid.UUID) uuid.UUID {
	t.Helper()
	var jobID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO extraction_jobs (
    tenant_id,
    case_id,
    step_id,
    document_id,
    schema_id,
    model_used,
    status,
    confidence,
    extracted_data,
    raw_response,
    processing_ms
) VALUES ($1, $2, 'extract', $3, $4, 'gpt-5.4', 'review', 0.72, '{}'::jsonb, '{}'::jsonb, 1200)
RETURNING id
`, tenantID, caseID, documentID, schemaID).Scan(&jobID); err != nil {
		t.Fatalf("insert extraction job: %v", err)
	}
	return jobID
}

func seedExtractionField(t *testing.T, ctx context.Context, db *sql.DB, jobID uuid.UUID) uuid.UUID {
	t.Helper()
	var fieldID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO extraction_fields (
    job_id,
    field_name,
    extracted_value,
    confidence,
    source_text,
    page_number,
    bbox_x,
    bbox_y,
    bbox_width,
    bbox_height,
    status
) VALUES ($1, 'company_number', 'A1234567', 0.72, 'A1234567', 1, 0.1, 0.2, 0.2, 0.05, 'extracted')
RETURNING id
`, jobID).Scan(&fieldID); err != nil {
		t.Fatalf("insert extraction field: %v", err)
	}
	return fieldID
}
