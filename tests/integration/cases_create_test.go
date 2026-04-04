package integration

import (
	"context"
	"testing"

	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/expressions"
)

func TestCasesIntegration_CreateGetAndValidation(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "c003-create")

	ctSvc := cases.NewCaseTypeService(db)
	ct, schemaErrs, err := ctSvc.RegisterCaseType(ctx, tenantID, principalID, "loan_application", testCaseSchema())
	if err != nil {
		t.Fatalf("register case type: %v", err)
	}
	if len(schemaErrs) > 0 {
		t.Fatalf("unexpected schema validation errors: %+v", schemaErrs)
	}

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{ID: "intake", Type: "human_task"},
		{ID: "review", Type: "human_task", DependsOn: []string{"intake"}},
	}}
	_, _ = seedPublishedWorkflow(t, ctx, db, tenantID, principalID, ct.Name, ast)

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	caseSvc := cases.NewCaseService(db, en)

	t.Run("create case with valid data generates number steps event and triggers dag", func(t *testing.T) {
		created, validation, err := caseSvc.CreateCase(ctx, tenantID, principalID, cases.CreateCaseRequest{
			CaseType: ct.Name,
			Priority: 2,
			Data: map[string]interface{}{
				"applicant": map[string]interface{}{"company_name": "Acme Ltd", "registration_number": "12345678"},
				"loan":      map[string]interface{}{"amount": 50000.0, "term_months": 24},
				"decision":  "pending",
			},
		})
		if err != nil {
			t.Fatalf("create case: %v", err)
		}
		if len(validation) > 0 {
			t.Fatalf("unexpected case validation errors: %+v", validation)
		}
		if created.CaseNumber == "" || created.CaseNumber[:3] != "LA-" {
			t.Fatalf("unexpected case number format: %s", created.CaseNumber)
		}

		steps := waitAndLoadCaseSteps(t, ctx, db, created.ID, 2)
		if steps[0].State == "pending" {
			t.Fatalf("expected first step to be activated by DAG evaluation, got pending")
		}

		full, err := caseSvc.GetCase(ctx, tenantID, created.ID)
		if err != nil {
			t.Fatalf("get case: %v", err)
		}
		if len(full.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %d", len(full.Steps))
		}
		if len(full.Events) == 0 {
			t.Fatalf("expected case_created audit event")
		}

		if _, err := db.ExecContext(ctx, `
INSERT INTO vault_documents (tenant_id, case_id, step_id, filename, mime_type, size_bytes, content_hash, storage_uri, uploaded_by)
VALUES ($1, $2, $3, 'id.pdf', 'application/pdf', 12, 'hash-1', 's3://bucket/id.pdf', $4)
`, tenantID, created.ID, "intake", principalID); err != nil {
			t.Fatalf("insert document: %v", err)
		}

		full, err = caseSvc.GetCase(ctx, tenantID, created.ID)
		if err != nil {
			t.Fatalf("get case after document insert: %v", err)
		}
		if len(full.Documents) != 1 {
			t.Fatalf("expected 1 document, got %d", len(full.Documents))
		}
	})

	t.Run("create case with invalid data returns structured validation errors", func(t *testing.T) {
		_, validation, err := caseSvc.CreateCase(ctx, tenantID, principalID, cases.CreateCaseRequest{
			CaseType: ct.Name,
			Data: map[string]interface{}{
				"applicant": map[string]interface{}{"company_name": "Acme Ltd", "registration_number": "abc"},
				"loan":      map[string]interface{}{"amount": 900000.0},
			},
		})
		if err != nil {
			t.Fatalf("create case with invalid payload returned error instead of validation details: %v", err)
		}
		if len(validation) == 0 {
			t.Fatal("expected validation errors for invalid payload")
		}
	})

	t.Run("create case with no published workflow fails", func(t *testing.T) {
		ctNoFlow, schemaErrs, err := ctSvc.RegisterCaseType(ctx, tenantID, principalID, "no_flow", testCaseSchema())
		if err != nil {
			t.Fatalf("register case type without workflow: %v", err)
		}
		if len(schemaErrs) > 0 {
			t.Fatalf("unexpected schema validation errors: %+v", schemaErrs)
		}

		_, validation, err := caseSvc.CreateCase(ctx, tenantID, principalID, cases.CreateCaseRequest{
			CaseType: ctNoFlow.Name,
			Data: map[string]interface{}{
				"applicant": map[string]interface{}{"company_name": "Acme Ltd", "registration_number": "12345678"},
				"loan":      map[string]interface{}{"amount": 50000.0},
			},
		})
		if err == nil {
			t.Fatal("expected error when no published workflow exists")
		}
		if len(validation) != 0 {
			t.Fatalf("expected no validation errors, got %+v", validation)
		}
	})
}
