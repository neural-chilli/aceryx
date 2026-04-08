package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/extraction"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

func TestExtractionStepExecutor_RoutingPaths(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "extract-exec-"+uuid.NewString()[:8])
	taskSvc := tasks.NewTaskService(db, nil, nil)
	exec := extraction.NewStepExecutor(db, taskSvc)

	t.Run("accepted path writes case patch and records accepted job", func(t *testing.T) {
		caseID, stepID := seedExtractionExecCase(t, ctx, db, tenantID, principalID, "accepted")
		schemaID := seedExtractionExecSchema(t, ctx, db, tenantID, "accepted_schema")
		docID := seedExtractionExecDocument(t, ctx, db, tenantID, caseID, principalID, map[string]any{
			"company_number": map[string]any{
				"value":       "12345678",
				"confidence":  0.95,
				"source_text": "12345678",
				"page_number": 1,
				"bbox_x":      0.1,
				"bbox_y":      0.2,
				"bbox_width":  0.2,
				"bbox_height": 0.05,
			},
		})
		setExtractionCaseAttachment(t, ctx, db, caseID, docID)

		cfg := mustJSON(t, map[string]any{
			"document_path":         "case.data.attachments[0].vault_id",
			"schema":                schemaID.String(),
			"model":                 "gpt-5.4",
			"auto_accept_threshold": 0.85,
			"review_threshold":      0.3,
			"output_path":           "case.data.extracted",
		})
		result, err := exec.Execute(ctx, caseID, stepID, cfg)
		if err != nil {
			t.Fatalf("execute extraction accepted path: %v", err)
		}
		if result == nil || !result.WritesCaseData || result.Outcome != "accept" {
			t.Fatalf("expected accepted extraction result with case-data patch, got %#v", result)
		}

		var status string
		if err := db.QueryRowContext(ctx, `
SELECT status
FROM extraction_jobs
WHERE case_id = $1
  AND step_id = $2
ORDER BY created_at DESC
LIMIT 1
`, caseID, stepID).Scan(&status); err != nil {
			t.Fatalf("query accepted extraction job: %v", err)
		}
		if status != "accepted" {
			t.Fatalf("expected accepted extraction job status, got %s", status)
		}
	})

	t.Run("review path creates review task and returns awaiting review", func(t *testing.T) {
		caseID, stepID := seedExtractionExecCase(t, ctx, db, tenantID, principalID, "review")
		schemaID := seedExtractionExecSchema(t, ctx, db, tenantID, "review_schema")
		docID := seedExtractionExecDocument(t, ctx, db, tenantID, caseID, principalID, map[string]any{
			"company_number": map[string]any{
				"value":       "12345678",
				"confidence":  0.7,
				"source_text": "12345678",
				"page_number": 1,
				"bbox_x":      0.1,
				"bbox_y":      0.2,
				"bbox_width":  0.2,
				"bbox_height": 0.05,
			},
		})
		setExtractionCaseAttachment(t, ctx, db, caseID, docID)

		cfg := mustJSON(t, map[string]any{
			"document_path":         "case.data.attachments[0].vault_id",
			"schema":                schemaID.String(),
			"model":                 "gpt-5.4",
			"auto_accept_threshold": 0.85,
			"review_threshold":      0.3,
			"output_path":           "case.data.extracted",
			"on_review":             map[string]any{"task_type": "extraction_review", "assignee_role": "case_worker", "sla_hours": 4},
		})
		_, err := exec.Execute(ctx, caseID, stepID, cfg)
		if !errors.Is(err, engine.ErrStepAwaitingReview) {
			t.Fatalf("expected ErrStepAwaitingReview, got %v", err)
		}

		var status string
		if err := db.QueryRowContext(ctx, `
SELECT status
FROM extraction_jobs
WHERE case_id = $1
  AND step_id = $2
ORDER BY created_at DESC
LIMIT 1
`, caseID, stepID).Scan(&status); err != nil {
			t.Fatalf("query review extraction job: %v", err)
		}
		if status != "review" {
			t.Fatalf("expected review extraction job status, got %s", status)
		}
	})

	t.Run("rejected path returns reject outcome and records rejected job", func(t *testing.T) {
		caseID, stepID := seedExtractionExecCase(t, ctx, db, tenantID, principalID, "reject")
		schemaID := seedExtractionExecSchema(t, ctx, db, tenantID, "reject_schema")
		docID := seedExtractionExecDocument(t, ctx, db, tenantID, caseID, principalID, map[string]any{
			"company_number": map[string]any{
				"value":      "12345678",
				"confidence": 0.1,
			},
		})
		setExtractionCaseAttachment(t, ctx, db, caseID, docID)

		cfg := mustJSON(t, map[string]any{
			"document_path":         "case.data.attachments[0].vault_id",
			"schema":                schemaID.String(),
			"model":                 "gpt-5.4",
			"auto_accept_threshold": 0.85,
			"review_threshold":      0.3,
			"output_path":           "case.data.extracted",
		})
		result, err := exec.Execute(ctx, caseID, stepID, cfg)
		if err != nil {
			t.Fatalf("execute extraction reject path: %v", err)
		}
		if result == nil || result.Outcome != "reject" {
			t.Fatalf("expected reject outcome, got %#v", result)
		}

		var status string
		if err := db.QueryRowContext(ctx, `
SELECT status
FROM extraction_jobs
WHERE case_id = $1
  AND step_id = $2
ORDER BY created_at DESC
LIMIT 1
`, caseID, stepID).Scan(&status); err != nil {
			t.Fatalf("query rejected extraction job: %v", err)
		}
		if status != "rejected" {
			t.Fatalf("expected rejected extraction job status, got %s", status)
		}
	})
}

func seedExtractionExecCase(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, suffix string) (uuid.UUID, string) {
	t.Helper()
	stepID := "extract_" + suffix
	ast := engine.WorkflowAST{
		Steps: []engine.WorkflowStep{
			{ID: stepID, Type: "extraction"},
		},
	}
	caseID := seedTaskCase(t, ctx, db, tenantID, principalID, "extract_exec_case_"+suffix, ast)
	return caseID, stepID
}

func seedExtractionExecSchema(t *testing.T, ctx context.Context, db *sql.DB, tenantID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var schemaID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO extraction_schemas (tenant_id, name, description, fields)
VALUES ($1, $2, $3, $4::jsonb)
RETURNING id
`, tenantID, name, "test schema", `[{"name":"company_number","type":"string","required":true}]`).Scan(&schemaID); err != nil {
		t.Fatalf("insert extraction schema: %v", err)
	}
	return schemaID
}

func seedExtractionExecDocument(t *testing.T, ctx context.Context, db *sql.DB, tenantID, caseID, principalID uuid.UUID, extracted map[string]any) uuid.UUID {
	t.Helper()
	rawExtracted, _ := json.Marshal(extracted)
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
    extracted_data,
    metadata
) VALUES ($1, $2, 'extract', 'application.pdf', 'application/pdf', 1234, $3, $4, $5, $6::jsonb, '{}'::jsonb)
RETURNING id
`, tenantID, caseID, fmt.Sprintf("hash-%s", uuid.NewString()[:8]), fmt.Sprintf("local://%s", uuid.NewString()[:8]), principalID, string(rawExtracted)).Scan(&documentID); err != nil {
		t.Fatalf("insert extraction document: %v", err)
	}
	return documentID
}

func setExtractionCaseAttachment(t *testing.T, ctx context.Context, db *sql.DB, caseID, documentID uuid.UUID) {
	t.Helper()
	payload := fmt.Sprintf(`{"attachments":[{"vault_id":"%s"}]}`, documentID.String())
	if _, err := db.ExecContext(ctx, `
UPDATE cases
SET data = $2::jsonb
WHERE id = $1
`, caseID, payload); err != nil {
		t.Fatalf("update case attachment payload: %v", err)
	}
}
