package integration

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/agents"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
	"github.com/neural-chilli/aceryx/internal/reports"
)

type fakeLLM struct {
	content string
}

func (f fakeLLM) ChatCompletion(_ context.Context, _ []agents.Message, _ *agents.ResponseFormat) (*agents.ChatResponse, error) {
	return &agents.ChatResponse{Content: f.content}, nil
}

func TestReportsIntegration_AskAndSavedLifecycle(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "reports-ask")
	tenantOther, principalOther := seedTenantAndPrincipal(t, ctx, db, "reports-other")
	caseID := seedAuditCase(t, ctx, db, tenantID, principalID, "report_case")
	if _, err := db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, events)
VALUES ($1, 'review', 'completed', '[]'::jsonb)
ON CONFLICT DO NOTHING
`, caseID); err != nil {
		t.Fatalf("insert case step for reporting: %v", err)
	}
	_ = seedAuditCase(t, ctx, db, tenantOther, principalOther, "report_case_other")

	svc := reports.NewService(db, fakeLLM{content: `{
  "sql":"SELECT COUNT(*) AS total FROM mv_report_cases",
  "title":"Total cases",
  "visualisation":"number",
  "columns":[{"key":"total","label":"Total","role":"measure"}]
}`})

	viewSchemas, err := svc.LoadViewSchemas(ctx)
	if err != nil {
		t.Fatalf("load view schemas: %v", err)
	}
	if len(viewSchemas) < 3 {
		t.Fatalf("expected seeded report view schemas, got %d", len(viewSchemas))
	}

	askResp, err := svc.Ask(ctx, tenantID, "How many cases?")
	if err != nil {
		t.Fatalf("ask reports: %v", err)
	}
	if askResp.RowCount != 1 {
		t.Fatalf("expected one row for count query, got %d", askResp.RowCount)
	}
	total := int64(0)
	switch v := askResp.Rows[0]["total"].(type) {
	case int64:
		total = v
	case float64:
		total = int64(v)
	case string:
		_ = json.Unmarshal([]byte(v), &total)
	}
	if total == 0 {
		t.Fatalf("expected tenant-scoped count to be > 0, got %v", askResp.Rows[0]["total"])
	}

	saved, err := svc.SaveReport(ctx, tenantID, principalID, reports.SaveReportRequest{
		Name:             "Total cases",
		Description:      "Count all cases",
		OriginalQuestion: "How many cases?",
		QuerySQL:         "SELECT COUNT(*) AS total FROM mv_report_cases",
		Visualisation:    "number",
		Columns:          []reports.ReportColumn{{Key: "total", Label: "Total", Role: "measure"}},
	})
	if err != nil {
		t.Fatalf("save report: %v", err)
	}

	runResp, err := svc.RunReport(ctx, tenantID, saved.ID)
	if err != nil {
		t.Fatalf("run saved report: %v", err)
	}
	if runResp.RowCount != 1 {
		t.Fatalf("expected one row from saved report run, got %d", runResp.RowCount)
	}

	updated, err := svc.UpdateReport(ctx, tenantID, principalID, saved.ID, reports.UpdateReportRequest{
		IsPublished: boolPtr(true),
		Pinned:      boolPtr(true),
	}, false)
	if err != nil {
		t.Fatalf("update report: %v", err)
	}
	if !updated.IsPublished || !updated.Pinned {
		t.Fatalf("expected report to be published and pinned, got %+v", updated)
	}

	mine, err := svc.ListReports(ctx, tenantID, principalID, "mine", false)
	if err != nil {
		t.Fatalf("list my reports: %v", err)
	}
	if len(mine) == 0 {
		t.Fatal("expected mine reports to include saved report")
	}

	published, err := svc.ListReports(ctx, tenantID, principalID, "published", false)
	if err != nil {
		t.Fatalf("list published reports: %v", err)
	}
	if len(published) == 0 {
		t.Fatal("expected published reports to include report")
	}

	if err := svc.DeleteReport(ctx, tenantID, principalID, saved.ID, false); err != nil {
		t.Fatalf("delete report: %v", err)
	}
	if _, err := svc.GetReport(ctx, tenantID, saved.ID); err == nil {
		t.Fatal("expected deleted report to be missing")
	}
}

func TestReportsIntegration_ExecutionLimitsAndReadOnlyRole(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "reports-limits")
	for i := 0; i < 120; i++ {
		_ = seedAuditCase(t, ctx, db, tenantID, principalID, "report_limit_case")
	}

	svc := reports.NewService(db, fakeLLM{content: `{}`})
	rows, _, err := svc.ExecuteSQL(ctx, tenantID, `SELECT c1.case_number, c2.case_number AS other_case FROM mv_report_cases c1 JOIN mv_report_cases c2 ON c1.tenant_id = c2.tenant_id`)
	if err != nil {
		t.Fatalf("execute large report query: %v", err)
	}
	if len(rows) != 10000 {
		t.Fatalf("expected capped rows at 10000, got %d", len(rows))
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Nanosecond)
	defer cancel()
	_, _, err = svc.ExecuteSQL(timeoutCtx, tenantID, `SELECT case_number FROM mv_report_cases`)
	if err == nil {
		t.Fatal("expected timeout/cancel to fail query")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx for reporter role test: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE aceryx_reporter`); err != nil {
		t.Fatalf("set local reporter role: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO saved_reports (tenant_id, created_by, name, query_sql) VALUES ($1, $2, 'x', 'SELECT 1')`, tenantID, principalID); err == nil {
		t.Fatal("expected aceryx_reporter to be unable to write")
	}
}

func TestReportsIntegration_ScheduledRunsUseSkipLocked(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}
	var tenantID, principalID uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT id FROM tenants WHERE slug='default'`).Scan(&tenantID); err != nil {
		t.Fatalf("load tenant: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id FROM principals WHERE tenant_id=$1 AND email='admin@localhost'`, tenantID).Scan(&principalID); err != nil {
		t.Fatalf("load principal: %v", err)
	}

	svc := reports.NewService(db, fakeLLM{content: `{}`})
	var sends int32
	svc.SetScheduleEmailSender(func(_ context.Context, _ uuid.UUID, _ reports.SavedReport, _ []map[string]any, _ []string) error {
		atomic.AddInt32(&sends, 1)
		return nil
	})

	saved, err := svc.SaveReport(ctx, tenantID, principalID, reports.SaveReportRequest{
		Name:          "Scheduled",
		QuerySQL:      "SELECT COUNT(*) AS total FROM mv_report_cases",
		Visualisation: "number",
	})
	if err != nil {
		t.Fatalf("save scheduled report: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE saved_reports SET schedule='daily', last_run_at = now() - interval '2 day' WHERE id = $1`, saved.ID); err != nil {
		t.Fatalf("set schedule due: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = svc.RunDueScheduledReports(ctx)
		done <- struct{}{}
	}()
	go func() {
		_ = svc.RunDueScheduledReports(ctx)
		done <- struct{}{}
	}()
	<-done
	<-done

	if atomic.LoadInt32(&sends) != 1 {
		t.Fatalf("expected one scheduled execution due to SKIP LOCKED, got %d", sends)
	}
}

func boolPtr(v bool) *bool { return &v }
