package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
)

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
