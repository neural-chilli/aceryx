package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/expressions"
	"github.com/neural-chilli/aceryx/internal/notify"
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

func TestCasesIntegration_ListSearchPatchCloseCancel(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "c003-list")
	ctSvc := cases.NewCaseTypeService(db)
	ct, schemaErrs, err := ctSvc.RegisterCaseType(ctx, tenantID, principalID, "loan_ops", testCaseSchema())
	if err != nil {
		t.Fatalf("register case type: %v", err)
	}
	if len(schemaErrs) > 0 {
		t.Fatalf("unexpected schema validation errors: %+v", schemaErrs)
	}
	_, _ = seedPublishedWorkflow(t, ctx, db, tenantID, principalID, ct.Name, engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "intake", Type: "human_task"}}})

	caseSvc := cases.NewCaseService(db, nil)

	mk := func(name string, amt float64, priority int) cases.Case {
		c, validation, err := caseSvc.CreateCase(ctx, tenantID, principalID, cases.CreateCaseRequest{
			CaseType: ct.Name,
			Priority: priority,
			Data: map[string]interface{}{
				"applicant": map[string]interface{}{"company_name": name, "registration_number": "12345678"},
				"loan":      map[string]interface{}{"amount": amt},
				"decision":  "pending",
			},
		})
		if err != nil {
			t.Fatalf("create case %s: %v", name, err)
		}
		if len(validation) > 0 {
			t.Fatalf("create case %s validation errors: %+v", name, validation)
		}
		return c
	}

	c1 := mk("Acme Corp", 11000, 1)
	c2 := mk("Beta Corp", 22000, 3)
	c3 := mk("Gamma Corp", 33000, 5)

	if _, err := db.ExecContext(ctx, `UPDATE cases SET status='in_progress', assigned_to=$2, priority=4 WHERE id=$1`, c2.ID, principalID); err != nil {
		t.Fatalf("update case c2 for list filtering: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE cases SET status='completed', priority=6 WHERE id=$1`, c3.ID); err != nil {
		t.Fatalf("update case c3 for list filtering: %v", err)
	}

	t.Run("list cases filters independently and combined", func(t *testing.T) {
		byStatus, err := caseSvc.ListCases(ctx, tenantID, cases.ListCasesFilter{Statuses: []string{"completed"}})
		if err != nil {
			t.Fatalf("list by status: %v", err)
		}
		if len(byStatus) != 1 || byStatus[0].ID != c3.ID {
			t.Fatalf("expected only completed case %s, got %+v", c3.ID, byStatus)
		}

		byAssigned, err := caseSvc.ListCases(ctx, tenantID, cases.ListCasesFilter{AssignedTo: &principalID})
		if err != nil {
			t.Fatalf("list by assigned_to: %v", err)
		}
		if len(byAssigned) != 1 || byAssigned[0].ID != c2.ID {
			t.Fatalf("expected only assigned case %s, got %+v", c2.ID, byAssigned)
		}

		combined, err := caseSvc.ListCases(ctx, tenantID, cases.ListCasesFilter{Statuses: []string{"in_progress"}, Priority: intPtr(4), AssignedTo: &principalID})
		if err != nil {
			t.Fatalf("list by combined filters: %v", err)
		}
		if len(combined) != 1 || combined[0].ID != c2.ID {
			t.Fatalf("expected only combined-filter case %s, got %+v", c2.ID, combined)
		}
	})

	t.Run("search full-text permission filtering and pagination", func(t *testing.T) {
		searchAll, err := caseSvc.SearchCases(ctx, tenantID, nil, cases.SearchFilter{Query: "Corp", PerPage: 1, Page: 1})
		if err != nil {
			t.Fatalf("search all page 1: %v", err)
		}
		if len(searchAll) != 1 {
			t.Fatalf("expected paginated result length 1, got %d", len(searchAll))
		}

		restricted, err := caseSvc.SearchCases(ctx, tenantID, []uuid.UUID{ct.ID}, cases.SearchFilter{Query: "Acme"})
		if err != nil {
			t.Fatalf("search restricted by allowed case type IDs: %v", err)
		}
		if len(restricted) == 0 {
			t.Fatal("expected restricted search to return matching rows")
		}

		otherTypeID := seedAdditionalCaseType(t, ctx, db, tenantID, principalID, "other_ops")
		_, _ = seedPublishedWorkflow(t, ctx, db, tenantID, principalID, "other_ops", engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "start", Type: "human_task"}}})
		otherSvc := cases.NewCaseService(db, nil)
		_, validation, err := otherSvc.CreateCase(ctx, tenantID, principalID, cases.CreateCaseRequest{
			CaseType: "other_ops",
			Data: map[string]interface{}{
				"applicant": map[string]interface{}{"company_name": "Acme Special", "registration_number": "12345678"},
				"loan":      map[string]interface{}{"amount": 12000.0},
				"decision":  "pending",
			},
		})
		if err != nil || len(validation) > 0 {
			t.Fatalf("create case in second case type: err=%v validation=%+v", err, validation)
		}

		restricted, err = caseSvc.SearchCases(ctx, tenantID, []uuid.UUID{otherTypeID}, cases.SearchFilter{Query: "Acme"})
		if err != nil {
			t.Fatalf("restricted search by other type: %v", err)
		}
		for _, row := range restricted {
			if row.CaseType != "other_ops" {
				t.Fatalf("permission filtered search leaked case type %s", row.CaseType)
			}
		}
	})

	t.Run("patch validates source and optimistic lock and deep merge", func(t *testing.T) {
		original, err := caseSvc.GetCase(ctx, tenantID, c1.ID)
		if err != nil {
			t.Fatalf("get case before patch: %v", err)
		}

		res, validation, err := caseSvc.UpdateCaseData(ctx, tenantID, c1.ID, principalID, map[string]interface{}{
			"applicant": map[string]interface{}{"company_name": "Acme Corporation Ltd"},
		}, original.Version)
		if err != nil {
			t.Fatalf("patch case: %v", err)
		}
		if len(validation) > 0 {
			t.Fatalf("unexpected patch validation errors: %+v", validation)
		}
		if got := res.Case.Data["applicant"].(map[string]interface{})["company_name"]; got != "Acme Corporation Ltd" {
			t.Fatalf("expected deep-merged company_name, got %#v", got)
		}

		_, validation, err = caseSvc.UpdateCaseData(ctx, tenantID, c1.ID, principalID, map[string]interface{}{"decision": "approved"}, res.Case.Version)
		if err != nil {
			t.Fatalf("patch agent sourced field should return validation errors, got err=%v", err)
		}
		if len(validation) == 0 {
			t.Fatal("expected validation error when patching agent-sourced field")
		}

		_, _, err = caseSvc.UpdateCaseData(ctx, tenantID, c1.ID, principalID, map[string]interface{}{"loan": map[string]interface{}{"amount": 11111.0}}, original.Version)
		if !errors.Is(err, engine.ErrCaseDataConflict) {
			t.Fatalf("expected optimistic lock conflict, got %v", err)
		}
	})

	t.Run("concurrent updates produce one conflict", func(t *testing.T) {
		current, err := caseSvc.GetCase(ctx, tenantID, c2.ID)
		if err != nil {
			t.Fatalf("get case for concurrent patch: %v", err)
		}

		var conflicts int32
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, _, err := caseSvc.UpdateCaseData(ctx, tenantID, c2.ID, principalID, map[string]interface{}{"loan": map[string]interface{}{"amount": float64(20000 + i)}}, current.Version)
				if errors.Is(err, engine.ErrCaseDataConflict) {
					atomic.AddInt32(&conflicts, 1)
				}
			}(i)
		}
		wg.Wait()
		if conflicts != 1 {
			t.Fatalf("expected one optimistic lock conflict, got %d", conflicts)
		}
	})

	t.Run("close case with active step fails", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `UPDATE case_steps SET state='active' WHERE case_id=$1`, c2.ID); err != nil {
			t.Fatalf("set active step: %v", err)
		}
		if err := caseSvc.CloseCase(ctx, tenantID, c2.ID, principalID, "done"); err == nil {
			t.Fatal("expected close case with active step to fail")
		}
	})

	t.Run("close case sends completion notification", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `UPDATE case_steps SET state='completed', completed_at=now() WHERE case_id=$1`, c2.ID); err != nil {
			t.Fatalf("complete steps before close: %v", err)
		}
		n := &caseNotifySpy{}
		svc := cases.NewCaseService(db, nil)
		svc.SetNotifier(n)
		if err := svc.CloseCase(ctx, tenantID, c2.ID, principalID, "all done"); err != nil {
			t.Fatalf("close case with notifier: %v", err)
		}
		if len(n.events) == 0 || n.events[0].Type != "case_completed" {
			t.Fatalf("expected case_completed notification, got %+v", n.events)
		}
	})

	t.Run("cancel case delegates to engine", func(t *testing.T) {
		stub := &stubCaseEngine{}
		svc := cases.NewCaseService(db, stub)
		n := &caseNotifySpy{}
		svc.SetNotifier(n)
		if err := svc.CancelCase(ctx, tenantID, c3.ID, principalID, "withdrawn"); err != nil {
			t.Fatalf("cancel case: %v", err)
		}
		if !stub.cancelCalled {
			t.Fatal("expected cancel to delegate to engine")
		}
		if stub.cancelReason != "withdrawn" {
			t.Fatalf("expected cancel reason 'withdrawn', got %q", stub.cancelReason)
		}
		if len(n.events) == 0 || n.events[0].Type != "case_cancelled" {
			t.Fatalf("expected case_cancelled notification, got %+v", n.events)
		}
	})
}

func TestCasesIntegration_CaseNumberGenerationAtomic(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "c003-num")
	ctSvc := cases.NewCaseTypeService(db)
	ct, schemaErrs, err := ctSvc.RegisterCaseType(ctx, tenantID, principalID, "seq_case", testCaseSchema())
	if err != nil {
		t.Fatalf("register case type: %v", err)
	}
	if len(schemaErrs) > 0 {
		t.Fatalf("schema errors: %+v", schemaErrs)
	}
	_, _ = seedPublishedWorkflow(t, ctx, db, tenantID, principalID, ct.Name, engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "s", Type: "human_task"}}})

	svc := cases.NewCaseService(db, nil)

	const workers = 10
	numbers := make(chan string, workers)
	errCh := make(chan error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			created, validation, err := svc.CreateCase(ctx, tenantID, principalID, cases.CreateCaseRequest{
				CaseType: ct.Name,
				Data: map[string]interface{}{
					"applicant": map[string]interface{}{"company_name": fmt.Sprintf("Org-%d", i), "registration_number": "12345678"},
					"loan":      map[string]interface{}{"amount": 10000.0 + float64(i)},
					"decision":  "pending",
				},
			})
			if err != nil {
				errCh <- err
				return
			}
			if len(validation) > 0 {
				errCh <- fmt.Errorf("validation errors: %+v", validation)
				return
			}
			numbers <- created.CaseNumber
		}(i)
	}
	wg.Wait()
	close(numbers)
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent create case failed: %v", err)
		}
	}

	vals := make([]int, 0, workers)
	for n := range numbers {
		var seq int
		if _, err := fmt.Sscanf(n, "SC-%06d", &seq); err != nil {
			t.Fatalf("parse generated case number %q: %v", n, err)
		}
		vals = append(vals, seq)
	}
	if len(vals) != workers {
		t.Fatalf("expected %d case numbers, got %d", workers, len(vals))
	}
	sort.Ints(vals)
	for i, v := range vals {
		expected := i + 1
		if v != expected {
			t.Fatalf("expected sequence %d at index %d, got %d (all=%v)", expected, i, v, vals)
		}
	}
}

func TestCasesIntegration_DashboardAndReports(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "c003-mi")
	ctSvc := cases.NewCaseTypeService(db)
	ct, schemaErrs, err := ctSvc.RegisterCaseType(ctx, tenantID, principalID, "mi_case", testCaseSchema())
	if err != nil {
		t.Fatalf("register case type: %v", err)
	}
	if len(schemaErrs) > 0 {
		t.Fatalf("schema errors: %+v", schemaErrs)
	}
	workflowID, _ := seedPublishedWorkflow(t, ctx, db, tenantID, principalID, ct.Name, engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "stage_a", Type: "human_task"}}})

	caseSvc := cases.NewCaseService(db, nil)
	reportSvc := cases.NewReportsService(db, 5*time.Minute)

	makeCase := func(name string, createdAt time.Time) uuid.UUID {
		var id uuid.UUID
		err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version, created_at, updated_at, priority)
VALUES ($1, $2, $3, 'open', $4::jsonb, $5, $6, 1, $7, $7, 1)
RETURNING id
`, tenantID, ct.ID, fmt.Sprintf("MI-%06d", time.Now().UnixNano()%1000000), `{"applicant":{"company_name":"Acme","registration_number":"12345678"},"loan":{"amount":10000},"decision":"pending"}`, principalID, workflowID, createdAt).Scan(&id)
		if err != nil {
			t.Fatalf("insert report case: %v", err)
		}
		return id
	}

	oldCase := makeCase("old", time.Now().AddDate(0, 0, -15))
	onTrackCase := makeCase("ontrack", time.Now().AddDate(0, 0, -1))
	warningCase := makeCase("warn", time.Now().AddDate(0, 0, -2))
	breachedCase := makeCase("breach", time.Now().AddDate(0, 0, -3))

	for _, id := range []uuid.UUID{oldCase, onTrackCase, warningCase, breachedCase} {
		if _, err := db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, assigned_to)
VALUES ($1, 'stage_a', 'active', $2)
`, id, principalID); err != nil {
			t.Fatalf("insert active step for dashboard: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE case_steps SET sla_deadline=now()+interval '48 hours' WHERE case_id=$1`, onTrackCase); err != nil {
		t.Fatalf("set on_track sla: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE case_steps SET sla_deadline=now()+interval '2 hours' WHERE case_id=$1`, warningCase); err != nil {
		t.Fatalf("set warning sla: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE case_steps SET sla_deadline=now()-interval '2 hours' WHERE case_id=$1`, breachedCase); err != nil {
		t.Fatalf("set breached sla: %v", err)
	}

	dashboard, err := caseSvc.Dashboard(ctx, tenantID, cases.DashboardFilter{SLAStatus: "breached", OlderThanDays: intPtr(1), SortBy: "created_at", SortDir: "ASC"})
	if err != nil {
		t.Fatalf("dashboard query: %v", err)
	}
	if len(dashboard) == 0 {
		t.Fatal("expected dashboard results")
	}
	foundBreached := false
	for _, row := range dashboard {
		if row.SLAStatus == "breached" {
			foundBreached = true
		}
	}
	if !foundBreached {
		t.Fatal("expected breached SLA row in dashboard")
	}

	completedAt := time.Now().Add(-2 * time.Hour)
	if _, err := db.ExecContext(ctx, `
UPDATE cases SET status='completed', updated_at=$2 WHERE id=$1
`, onTrackCase, completedAt); err != nil {
		t.Fatalf("set completed case status: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE case_steps SET state='completed', completed_at=$2 WHERE case_id=$1
`, onTrackCase, completedAt); err != nil {
		t.Fatalf("complete step for sla compliance: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE cases SET status='cancelled', updated_at=now() WHERE id=$1
`, warningCase); err != nil {
		t.Fatalf("set cancelled case status: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO case_events (case_id, event_type, actor_id, actor_type, action, data, prev_event_hash, event_hash)
VALUES
($1, 'case', $2, 'agent', 'updated', '{}'::jsonb, 'p1', 'h1'),
($1, 'case', $2, 'human', 'updated', '{}'::jsonb, 'h1', 'h2')
`, onTrackCase, principalID); err != nil {
		t.Fatalf("insert decision events: %v", err)
	}

	if err := reportSvc.RefreshMaterializedViews(ctx); err != nil {
		t.Fatalf("refresh materialized views: %v", err)
	}

	summary, err := reportSvc.CasesSummary(ctx, tenantID, 12)
	if err != nil {
		t.Fatalf("cases summary: %v", err)
	}
	if len(summary) == 0 {
		t.Fatal("expected non-empty cases summary")
	}

	ageing, err := reportSvc.Ageing(ctx, tenantID, []int{7, 14, 30})
	if err != nil {
		t.Fatalf("ageing report: %v", err)
	}
	if len(ageing) != 4 {
		t.Fatalf("expected 4 ageing buckets, got %d", len(ageing))
	}

	sla, err := reportSvc.SLACompliance(ctx, tenantID, 12)
	if err != nil {
		t.Fatalf("sla compliance report: %v", err)
	}
	if len(sla) == 0 {
		t.Fatal("expected non-empty sla compliance report")
	}

	stages, err := reportSvc.CasesByStage(ctx, tenantID, ct.Name)
	if err != nil {
		t.Fatalf("cases by stage report: %v", err)
	}
	if len(stages) == 0 {
		t.Fatal("expected non-empty stage distribution report")
	}

	workload, err := reportSvc.Workload(ctx, tenantID)
	if err != nil {
		t.Fatalf("workload report: %v", err)
	}
	if len(workload) == 0 {
		t.Fatal("expected workload report rows")
	}

	decisions, err := reportSvc.Decisions(ctx, tenantID, 12)
	if err != nil {
		t.Fatalf("decisions report: %v", err)
	}
	if len(decisions) == 0 {
		t.Fatal("expected decisions report rows")
	}
}

func seedTenantAndPrincipal(t *testing.T, ctx context.Context, db *sql.DB, slug string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	var tenantID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO tenants (name, slug, branding, terminology, settings)
VALUES ($1, $2, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb)
RETURNING id
`, "Tenant "+slug, slug).Scan(&tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	var principalID uuid.UUID
	err = db.QueryRowContext(ctx, `
INSERT INTO principals (tenant_id, type, name, email, status)
VALUES ($1, 'human', $2, $3, 'active')
RETURNING id
`, tenantID, "Principal "+slug, slug+"@example.com").Scan(&principalID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}
	return tenantID, principalID
}

func seedPublishedWorkflow(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, caseTypeName string, ast engine.WorkflowAST) (uuid.UUID, int) {
	t.Helper()
	var workflowID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO workflows (tenant_id, name, case_type, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id
`, tenantID, "workflow-"+uuid.NewString()[:8], caseTypeName, principalID).Scan(&workflowID)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	rawAST, err := json.Marshal(ast)
	if err != nil {
		t.Fatalf("marshal ast: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workflow_versions (workflow_id, version, status, ast, yaml_source, created_by, published_at)
VALUES ($1, 1, 'published', $2::jsonb, '', $3, now())
`, workflowID, string(rawAST), principalID); err != nil {
		t.Fatalf("insert workflow version: %v", err)
	}
	return workflowID, 1
}

func seedAdditionalCaseType(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	rawSchema, err := json.Marshal(testCaseSchema())
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var id uuid.UUID
	err = db.QueryRowContext(ctx, `
INSERT INTO case_types (tenant_id, name, version, schema, status, created_by)
VALUES ($1, $2, 1, $3::jsonb, 'active', $4)
RETURNING id
`, tenantID, name, string(rawSchema), principalID).Scan(&id)
	if err != nil {
		t.Fatalf("insert additional case type: %v", err)
	}
	return id
}

func testCaseSchema() cases.CaseTypeSchema {
	minAmount := 5000.0
	maxAmount := 500000.0
	minName := 2
	maxName := 120
	return cases.CaseTypeSchema{Fields: map[string]cases.SchemaField{
		"applicant": {
			Type: "object",
			Properties: map[string]cases.SchemaField{
				"company_name":        {Type: "string", Required: true, MinLength: &minName, MaxLength: &maxName},
				"registration_number": {Type: "string", Pattern: "^[0-9]{8}$"},
			},
		},
		"loan": {
			Type: "object",
			Properties: map[string]cases.SchemaField{
				"amount":      {Type: "number", Required: true, Min: &minAmount, Max: &maxAmount},
				"term_months": {Type: "integer"},
			},
		},
		"decision": {
			Type:   "string",
			Source: "agent",
			Enum:   []interface{}{"pending", "approved", "rejected"},
		},
	}}
}

func waitAndLoadCaseSteps(t *testing.T, ctx context.Context, db *sql.DB, caseID uuid.UUID, expected int) []cases.CaseStep {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := db.QueryContext(ctx, `
SELECT id, step_id, state, started_at, completed_at, result, events, error, assigned_to, sla_deadline, retry_count, draft_data, metadata
FROM case_steps
WHERE case_id=$1
ORDER BY step_id
`, caseID)
		if err != nil {
			t.Fatalf("query steps: %v", err)
		}
		out := make([]cases.CaseStep, 0)
		for rows.Next() {
			var st cases.CaseStep
			if err := rows.Scan(&st.ID, &st.StepID, &st.State, &st.StartedAt, &st.CompletedAt, &st.Result, &st.Events, &st.Error, &st.AssignedTo, &st.SLADeadline, &st.RetryCount, &st.DraftData, &st.Metadata); err != nil {
				_ = rows.Close()
				t.Fatalf("scan steps: %v", err)
			}
			out = append(out, st)
		}
		_ = rows.Close()
		if len(out) >= expected {
			return out
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("steps did not reach expected count=%d in time", expected)
	return nil
}

type stubCaseEngine struct {
	cancelCalled bool
	cancelReason string
}

type caseNotifySpy struct {
	events []notify.NotifyEvent
}

func (s *caseNotifySpy) Notify(_ context.Context, event notify.NotifyEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubCaseEngine) EvaluateDAG(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (s *stubCaseEngine) CancelCase(_ context.Context, _ uuid.UUID, _ uuid.UUID, reason string) error {
	s.cancelCalled = true
	s.cancelReason = reason
	return nil
}

func intPtr(v int) *int { return &v }
