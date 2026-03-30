package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
	"github.com/neural-chilli/aceryx/internal/vault"
)

func TestVaultIntegration_APIUploadListDownloadDeleteSignedURL(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}
	tenantID, adminID, adminEmail := fetchDefaultAdmin(t, ctx, db)
	caseID := seedVaultCase(t, ctx, db, tenantID, adminID, "vault_case_api")

	vaultRoot := t.TempDir()
	t.Setenv("ACERYX_VAULT_ROOT", vaultRoot)
	t.Setenv("ACERYX_JWT_SECRET", "vault-secret")
	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")

	uploadReq := newUploadRequest(t, "/cases/"+caseID.String()+"/documents", "evidence.txt", "text/plain", []byte("sample evidence"), `{"document_type":"evidence"}`)
	uploadReq.Header.Set("Authorization", "Bearer "+login.Token)
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)
	if uploadW.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", uploadW.Code, uploadW.Body.String())
	}
	var uploaded vault.Document
	if err := json.Unmarshal(uploadW.Body.Bytes(), &uploaded); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if uploaded.DisplayMode != "inline" {
		t.Fatalf("expected inline mode, got %s", uploaded.DisplayMode)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/cases/"+caseID.String()+"/documents", nil)
	listReq.Header.Set("Authorization", "Bearer "+login.Token)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listW.Code, listW.Body.String())
	}
	var listed []vault.Document
	if err := json.Unmarshal(listW.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 listed doc, got %d", len(listed))
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/cases/"+caseID.String()+"/documents/"+uploaded.ID.String(), nil)
	downloadReq.Header.Set("Authorization", "Bearer "+login.Token)
	downloadW := httptest.NewRecorder()
	router.ServeHTTP(downloadW, downloadReq)
	if downloadW.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", downloadW.Code, downloadW.Body.String())
	}
	if ct := downloadW.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content-type, got %s", ct)
	}
	if !strings.Contains(downloadW.Body.String(), "sample evidence") {
		t.Fatalf("unexpected download body: %s", downloadW.Body.String())
	}

	signedReq := httptest.NewRequest(http.MethodGet, "/cases/"+caseID.String()+"/documents/"+uploaded.ID.String()+"/signed-url", nil)
	signedReq.Header.Set("Authorization", "Bearer "+login.Token)
	signedW := httptest.NewRecorder()
	router.ServeHTTP(signedW, signedReq)
	if signedW.Code != http.StatusOK {
		t.Fatalf("signed-url status=%d body=%s", signedW.Code, signedW.Body.String())
	}
	var signed vault.SignedDocumentURL
	if err := json.Unmarshal(signedW.Body.Bytes(), &signed); err != nil {
		t.Fatalf("decode signed-url response: %v", err)
	}
	if signed.URL == "" {
		t.Fatal("expected signed url")
	}

	signedDownloadReq := httptest.NewRequest(http.MethodGet, signed.URL, nil)
	signedDownloadW := httptest.NewRecorder()
	router.ServeHTTP(signedDownloadW, signedDownloadReq)
	if signedDownloadW.Code != http.StatusOK {
		t.Fatalf("signed download status=%d body=%s", signedDownloadW.Code, signedDownloadW.Body.String())
	}

	wrongCaseReq := httptest.NewRequest(http.MethodGet, "/cases/"+uuid.NewString()+"/documents/"+uploaded.ID.String(), nil)
	wrongCaseReq.Header.Set("Authorization", "Bearer "+login.Token)
	wrongCaseW := httptest.NewRecorder()
	router.ServeHTTP(wrongCaseW, wrongCaseReq)
	if wrongCaseW.Code != http.StatusNotFound {
		t.Fatalf("expected wrong case 404, got %d", wrongCaseW.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/cases/"+caseID.String()+"/documents/"+uploaded.ID.String(), nil)
	deleteReq.Header.Set("Authorization", "Bearer "+login.Token)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteW.Code, deleteW.Body.String())
	}

	downloadAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/cases/"+caseID.String()+"/documents/"+uploaded.ID.String(), nil)
	downloadAfterDeleteReq.Header.Set("Authorization", "Bearer "+login.Token)
	downloadAfterDeleteW := httptest.NewRecorder()
	router.ServeHTTP(downloadAfterDeleteW, downloadAfterDeleteReq)
	if downloadAfterDeleteW.Code != http.StatusNotFound {
		t.Fatalf("expected deleted doc 404, got %d", downloadAfterDeleteW.Code)
	}

	var auditDownloads int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM case_events WHERE case_id = $1 AND event_type = 'document' AND action = 'downloaded'`, caseID).Scan(&auditDownloads); err != nil {
		t.Fatalf("count document download events: %v", err)
	}
	if auditDownloads == 0 {
		t.Fatal("expected document.downloaded audit events")
	}
}

func TestVaultIntegration_DedupAndOrphanCleanupAndConcurrentUpload(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	root := t.TempDir()
	store := vault.NewLocalVaultStore(root, "secret")
	svc := vault.NewService(db, store, time.Hour)

	tenantA, principalA := seedTenantAndPrincipal(t, ctx, db, "vault-tenant-a")
	tenantB, principalB := seedTenantAndPrincipal(t, ctx, db, "vault-tenant-b")
	caseA1 := seedVaultCase(t, ctx, db, tenantA, principalA, "vault_case_a1")
	caseA2 := seedVaultCase(t, ctx, db, tenantA, principalA, "vault_case_a2")
	caseB1 := seedVaultCase(t, ctx, db, tenantB, principalB, "vault_case_b1")
	data := []byte("same-document")

	docA1, err := svc.Upload(ctx, tenantA, vault.UploadInput{CaseID: caseA1, Filename: "same.pdf", MimeType: "application/pdf", Data: data, UploadedBy: principalA})
	if err != nil {
		t.Fatalf("upload a1: %v", err)
	}
	docA2, err := svc.Upload(ctx, tenantA, vault.UploadInput{CaseID: caseA2, Filename: "same.pdf", MimeType: "application/pdf", Data: data, UploadedBy: principalA})
	if err != nil {
		t.Fatalf("upload a2: %v", err)
	}
	if docA1.ContentHash != docA2.ContentHash {
		t.Fatalf("expected same hash for tenant dedup")
	}

	var distinctURIA int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT storage_uri) FROM vault_documents WHERE tenant_id = $1 AND content_hash = $2`, tenantA, docA1.ContentHash).Scan(&distinctURIA); err != nil {
		t.Fatalf("count distinct uri in tenant A: %v", err)
	}
	if distinctURIA != 1 {
		t.Fatalf("expected one physical uri in tenant A, got %d", distinctURIA)
	}

	docB1, err := svc.Upload(ctx, tenantB, vault.UploadInput{CaseID: caseB1, Filename: "same.pdf", MimeType: "application/pdf", Data: data, UploadedBy: principalB})
	if err != nil {
		t.Fatalf("upload b1: %v", err)
	}
	if docA1.ContentHash != docB1.ContentHash {
		t.Fatalf("expected same hash for identical content across tenants")
	}
	var uriA, uriB string
	if err := db.QueryRowContext(ctx, `SELECT storage_uri FROM vault_documents WHERE id = $1`, docA1.ID).Scan(&uriA); err != nil {
		t.Fatalf("load uriA: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT storage_uri FROM vault_documents WHERE id = $1`, docB1.ID).Scan(&uriB); err != nil {
		t.Fatalf("load uriB: %v", err)
	}
	if uriA == uriB {
		t.Fatalf("expected cross-tenant storage isolation")
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = svc.Upload(ctx, tenantA, vault.UploadInput{CaseID: caseA1, Filename: "concurrent.txt", MimeType: "text/plain", Data: []byte("same-concurrent"), UploadedBy: principalA})
		}(i)
	}
	wg.Wait()
	var rowCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE tenant_id = $1 AND case_id = $2 AND filename = 'concurrent.txt'`, tenantA, caseA1).Scan(&rowCount); err != nil {
		t.Fatalf("count concurrent rows: %v", err)
	}
	if rowCount != 10 {
		t.Fatalf("expected 10 metadata rows for concurrent upload, got %d", rowCount)
	}
	var distinctConcurrentURI int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT storage_uri) FROM vault_documents WHERE tenant_id = $1 AND case_id = $2 AND filename = 'concurrent.txt'`, tenantA, caseA1).Scan(&distinctConcurrentURI); err != nil {
		t.Fatalf("count distinct concurrent uri: %v", err)
	}
	if distinctConcurrentURI != 1 {
		t.Fatalf("expected deduped single physical file for concurrent upload, got %d", distinctConcurrentURI)
	}

	if err := svc.Delete(ctx, tenantA, caseA1, docA1.ID, principalA); err != nil {
		t.Fatalf("delete docA1: %v", err)
	}
	filesDeleted, _, err := svc.OrphanCleanup(ctx, &tenantA)
	if err != nil {
		t.Fatalf("orphan cleanup tenant A with live ref: %v", err)
	}
	if filesDeleted != 0 {
		t.Fatalf("expected no delete with live reference, got %d", filesDeleted)
	}

	if err := svc.Delete(ctx, tenantA, caseA2, docA2.ID, principalA); err != nil {
		t.Fatalf("delete docA2: %v", err)
	}
	filesDeleted, _, err = svc.OrphanCleanup(ctx, &tenantA)
	if err != nil {
		t.Fatalf("orphan cleanup tenant A after full delete: %v", err)
	}
	if filesDeleted == 0 {
		t.Fatal("expected orphan cleanup to delete physical file")
	}
}

func TestVaultIntegration_GDPR_ErasureAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	svc := vault.NewService(db, vault.NewLocalVaultStore(t.TempDir(), "secret"), time.Hour)
	tenantA, principalA := seedTenantAndPrincipal(t, ctx, db, "vault-erasure-a")
	tenantB, principalB := seedTenantAndPrincipal(t, ctx, db, "vault-erasure-b")
	caseA := seedVaultCase(t, ctx, db, tenantA, principalA, "vault_erasure_case")
	caseB := seedVaultCase(t, ctx, db, tenantB, principalB, "vault_erasure_case_b")

	docA, err := svc.Upload(ctx, tenantA, vault.UploadInput{CaseID: caseA, Filename: "pii.txt", MimeType: "text/plain", Data: []byte("pii-data"), UploadedBy: principalA})
	if err != nil {
		t.Fatalf("upload docA: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE case_steps SET events = '[{"type":"llm_call","context_snapshot":{"pii":"x"}}]'::jsonb WHERE case_id = $1`, caseA); err != nil {
		t.Fatalf("seed case events snapshot: %v", err)
	}

	if _, _, err := svc.Download(ctx, tenantB, caseA, docA.ID, principalB); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected tenant isolation not found, got %v", err)
	}

	if err := svc.Erase(ctx, tenantA, vault.ErasureRequest{CaseID: &caseA}, principalA); err != nil {
		t.Fatalf("erase case A: %v", err)
	}

	var liveCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE case_id = $1 AND deleted_at IS NULL`, caseA).Scan(&liveCount); err != nil {
		t.Fatalf("count live documents after erasure: %v", err)
	}
	if liveCount != 0 {
		t.Fatalf("expected no live docs after erasure, got %d", liveCount)
	}
	var eventsJSON []byte
	if err := db.QueryRowContext(ctx, `SELECT events FROM case_steps WHERE case_id = $1 LIMIT 1`, caseA).Scan(&eventsJSON); err != nil {
		t.Fatalf("load case_steps events after erasure: %v", err)
	}
	if string(eventsJSON) != "[]" {
		t.Fatalf("expected purged case_steps.events, got %s", string(eventsJSON))
	}

	// Ensure tenant B unaffected.
	docB, err := svc.Upload(ctx, tenantB, vault.UploadInput{CaseID: caseB, Filename: "safe.txt", MimeType: "text/plain", Data: []byte("safe"), UploadedBy: principalB})
	if err != nil {
		t.Fatalf("upload docB: %v", err)
	}
	if _, _, err := svc.Download(ctx, tenantB, caseB, docB.ID, principalB); err != nil {
		t.Fatalf("tenant B document should remain accessible: %v", err)
	}
}

func newUploadRequest(t *testing.T, path, filename, mimeType string, data []byte, metadataJSON string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write file payload: %v", err)
	}
	if metadataJSON != "" {
		if err := writer.WriteField("metadata", metadataJSON); err != nil {
			t.Fatalf("write metadata field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if mimeType != "" {
		req.Header.Set("X-File-Content-Type", mimeType)
	}
	return req
}

func fetchDefaultAdmin(t *testing.T, ctx context.Context, db *sql.DB) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	var tenantID uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT id FROM tenants WHERE slug = 'default'`).Scan(&tenantID); err != nil {
		t.Fatalf("load default tenant id: %v", err)
	}
	var adminID uuid.UUID
	var adminEmail string
	if err := db.QueryRowContext(ctx, `SELECT id, email FROM principals WHERE tenant_id = $1 AND email = 'admin@localhost'`, tenantID).Scan(&adminID, &adminEmail); err != nil {
		t.Fatalf("load default admin principal: %v", err)
	}
	return tenantID, adminID, adminEmail
}

func seedVaultCase(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, caseTypeName string) uuid.UUID {
	t.Helper()
	ctSvc := cases.NewCaseTypeService(db)
	schema := cases.CaseTypeSchema{Fields: map[string]cases.SchemaField{"name": {Type: "string", Required: true}}}
	_, _, _ = ctSvc.RegisterCaseType(ctx, tenantID, principalID, caseTypeName, schema)
	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "start", Type: "rule"}}}
	workflowID, _ := seedPublishedWorkflow(t, ctx, db, tenantID, principalID, caseTypeName, ast)
	var caseTypeID uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT id FROM case_types WHERE tenant_id = $1 AND name = $2 ORDER BY version DESC LIMIT 1`, tenantID, caseTypeName).Scan(&caseTypeID); err != nil {
		t.Fatalf("load vault case type id: %v", err)
	}
	var caseID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'open', '{"name":"Vault"}'::jsonb, $4, $5, 1)
RETURNING id
`, tenantID, caseTypeID, "VLT-"+uuid.NewString()[:8], principalID, workflowID).Scan(&caseID); err != nil {
		t.Fatalf("insert vault case: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO case_steps (case_id, step_id, state, events, retry_count) VALUES ($1, 'start', 'pending', '[]'::jsonb, 0)`, caseID); err != nil {
		t.Fatalf("insert initial case step: %v", err)
	}
	return caseID
}

func TestVaultIntegration_UploadSizeLimit(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()
	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}
	tenantID, adminID, adminEmail := fetchDefaultAdmin(t, ctx, db)
	caseID := seedVaultCase(t, ctx, db, tenantID, adminID, "vault_case_limit")
	t.Setenv("ACERYX_VAULT_ROOT", filepath.Join(t.TempDir(), "vault"))
	t.Setenv("ACERYX_JWT_SECRET", "vault-secret")
	t.Setenv("ACERYX_MAX_DOCUMENT_SIZE", "10")
	router := api.NewRouterWithServices(db, nil)
	login := loginViaAPI(t, router, tenantID, adminEmail, "admin")

	req := newUploadRequest(t, "/cases/"+caseID.String()+"/documents", "large.txt", "text/plain", []byte("01234567890"), "")
	req.Header.Set("Authorization", "Bearer "+login.Token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for size limit, got %d body=%s", w.Code, w.Body.String())
	}
}
