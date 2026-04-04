package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api"
	"github.com/neural-chilli/aceryx/internal/audit"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/expressions"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
)

func TestAuditIntegration_RecordVerifyAndCallbacks(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, principalID := seedTenantAndPrincipal(t, ctx, db, "audit-int")
	caseID := seedAuditCase(t, ctx, db, tenantID, principalID, "audit_case_int")

	svc := audit.NewService(db)

	for i := 0; i < 10; i++ {
		if err := appendAuditEvent(ctx, svc, db, caseID, "step", "completed", map[string]any{"idx": i}); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	verify, err := svc.VerifyCaseChain(ctx, caseID)
	if err != nil {
		t.Fatalf("verify chain: %v", err)
	}
	if !verify.Valid || verify.EventCount != 10 {
		t.Fatalf("expected valid chain with 10 events, got %+v", verify)
	}

	if _, err := db.ExecContext(ctx, `
UPDATE case_events
SET data = '{"tampered":true}'::jsonb
WHERE id = (
    SELECT id FROM case_events WHERE case_id = $1 ORDER BY created_at, id OFFSET 4 LIMIT 1
)
`, caseID); err != nil {
		t.Fatalf("tamper event: %v", err)
	}
	verifyAfterTamper, err := svc.VerifyCaseChain(ctx, caseID)
	if err != nil {
		t.Fatalf("verify chain after tamper: %v", err)
	}
	if verifyAfterTamper.Valid {
		t.Fatalf("expected tampered chain to be invalid: %+v", verifyAfterTamper)
	}

	events, err := svc.ListCaseEvents(ctx, caseID, audit.ListFilter{EventType: "step", Action: "completed", Page: 1, PerPage: 5})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("expected pagination to return 5 events, got %d", len(events))
	}

	jsonExport, err := svc.ExportCaseEventsJSON(ctx, caseID)
	if err != nil {
		t.Fatalf("json export: %v", err)
	}
	var exported []audit.Event
	if err := json.Unmarshal(jsonExport, &exported); err != nil {
		t.Fatalf("decode json export: %v", err)
	}
	if len(exported) != 10 {
		t.Fatalf("expected 10 exported events, got %d", len(exported))
	}

	csvExport, err := svc.ExportCaseEventsCSV(ctx, caseID)
	if err != nil {
		t.Fatalf("csv export: %v", err)
	}
	reader := csv.NewReader(bytes.NewReader(csvExport))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("decode csv export: %v", err)
	}
	if len(rows) != 11 {
		t.Fatalf("expected 11 csv rows (header + 10 events), got %d", len(rows))
	}

	t.Run("event rollback does not persist", func(t *testing.T) {
		var before int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM case_events WHERE case_id = $1`, caseID).Scan(&before); err != nil {
			t.Fatalf("count events before rollback: %v", err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin rollback tx: %v", err)
		}
		if _, err := tx.ExecContext(ctx, `SELECT id FROM cases WHERE id = $1 FOR UPDATE`, caseID); err != nil {
			t.Fatalf("lock case row for rollback: %v", err)
		}
		if err := svc.RecordCaseEventTx(ctx, tx, caseID, "", "case", principalID, "human", "updated", map[string]any{"rollback": true}); err != nil {
			t.Fatalf("record rollback event: %v", err)
		}
		if err := svc.RollbackTx(tx); err != nil {
			t.Fatalf("rollback tx: %v", err)
		}
		var after int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM case_events WHERE case_id = $1`, caseID).Scan(&after); err != nil {
			t.Fatalf("count events after rollback: %v", err)
		}
		if before != after {
			t.Fatalf("expected rollback to keep event count unchanged before=%d after=%d", before, after)
		}
	})

	t.Run("post-commit callback fires only on commit", func(t *testing.T) {
		ch := make(chan audit.Event, 2)
		svc.OnCommitted(func(e audit.Event) { ch <- e })

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin callback commit tx: %v", err)
		}
		if _, err := tx.ExecContext(ctx, `SELECT id FROM cases WHERE id = $1 FOR UPDATE`, caseID); err != nil {
			t.Fatalf("lock case row commit callback: %v", err)
		}
		if err := svc.RecordCaseEventTx(ctx, tx, caseID, "", "case", principalID, "human", "updated", map[string]any{"callback": "commit"}); err != nil {
			t.Fatalf("record callback commit event: %v", err)
		}
		if err := svc.CommitTx(tx); err != nil {
			t.Fatalf("commit callback tx: %v", err)
		}
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("expected post-commit callback to fire")
		}

		tx2, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin callback rollback tx: %v", err)
		}
		if _, err := tx2.ExecContext(ctx, `SELECT id FROM cases WHERE id = $1 FOR UPDATE`, caseID); err != nil {
			t.Fatalf("lock case row rollback callback: %v", err)
		}
		if err := svc.RecordCaseEventTx(ctx, tx2, caseID, "", "case", principalID, "human", "updated", map[string]any{"callback": "rollback"}); err != nil {
			t.Fatalf("record callback rollback event: %v", err)
		}
		if err := svc.RollbackTx(tx2); err != nil {
			t.Fatalf("rollback callback tx: %v", err)
		}
		select {
		case ev := <-ch:
			t.Fatalf("did not expect callback after rollback, got %+v", ev)
		case <-time.After(300 * time.Millisecond):
		}
	})
}

func TestAuditIntegration_ConcurrentEngineEventRecordingAndEndpoints(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		t.Fatalf("seed default data: %v", err)
	}

	var tenantID, principalID uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT id FROM tenants WHERE slug = 'default'`).Scan(&tenantID); err != nil {
		t.Fatalf("load default tenant: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id FROM principals WHERE tenant_id = $1 AND email = 'admin@localhost'`, tenantID).Scan(&principalID); err != nil {
		t.Fatalf("load default admin principal: %v", err)
	}

	caseID := seedAuditCase(t, ctx, db, tenantID, principalID, "audit_case_engine")
	if _, err := db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, events)
VALUES ($1, 'a', 'active', '[]'::jsonb), ($1, 'b', 'active', '[]'::jsonb)
ON CONFLICT DO NOTHING
`, caseID); err != nil {
		t.Fatalf("insert active steps: %v", err)
	}

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = en.CompleteStep(ctx, caseID, "a", &engine.StepResult{Outcome: "ok"})
	}()
	go func() {
		defer wg.Done()
		_ = en.CompleteStep(ctx, caseID, "b", &engine.StepResult{Outcome: "ok"})
	}()
	wg.Wait()

	var completedCount int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM case_events
WHERE case_id = $1 AND event_type = 'step' AND action = 'completed'
`, caseID).Scan(&completedCount); err != nil {
		t.Fatalf("count completed step events: %v", err)
	}
	if completedCount != 2 {
		t.Fatalf("expected 2 completed step events, got %d", completedCount)
	}

	genesis := audit.GenesisHash(caseID)
	var genesisPrevCount int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM case_events
WHERE case_id = $1 AND prev_event_hash = $2
`, caseID, genesis).Scan(&genesisPrevCount); err != nil {
		t.Fatalf("count genesis prev hash events: %v", err)
	}
	if genesisPrevCount != 1 {
		t.Fatalf("expected exactly one event to reference genesis hash, got %d", genesisPrevCount)
	}

	router := api.NewRouterWithServices(db, en)
	login := loginViaAPI(t, router, tenantID, "admin@localhost", "admin")

	verifyReq := httptest.NewRequest(http.MethodPost, "/cases/"+caseID.String()+"/events/verify", nil)
	verifyReq.Header.Set("Authorization", "Bearer "+login.Token)
	verifyW := httptest.NewRecorder()
	router.ServeHTTP(verifyW, verifyReq)
	if verifyW.Code != http.StatusOK {
		t.Fatalf("verify endpoint status=%d body=%s", verifyW.Code, verifyW.Body.String())
	}
	var verifyResp audit.VerifyResult
	if err := json.Unmarshal(verifyW.Body.Bytes(), &verifyResp); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !verifyResp.Valid {
		t.Fatalf("expected verify endpoint valid=true, got %+v", verifyResp)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/cases/"+caseID.String()+"/events?page=1&per_page=1&event_type=step&action=completed", nil)
	listReq.Header.Set("Authorization", "Bearer "+login.Token)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list endpoint status=%d body=%s", listW.Code, listW.Body.String())
	}
	var page []audit.Event
	if err := json.Unmarshal(listW.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("expected list page size 1, got %d", len(page))
	}

	exportJSONReq := httptest.NewRequest(http.MethodGet, "/cases/"+caseID.String()+"/events/export?format=json", nil)
	exportJSONReq.Header.Set("Authorization", "Bearer "+login.Token)
	exportJSONW := httptest.NewRecorder()
	router.ServeHTTP(exportJSONW, exportJSONReq)
	if exportJSONW.Code != http.StatusOK {
		t.Fatalf("export json endpoint status=%d body=%s", exportJSONW.Code, exportJSONW.Body.String())
	}
	var exportedJSON []audit.Event
	if err := json.Unmarshal(exportJSONW.Body.Bytes(), &exportedJSON); err != nil {
		t.Fatalf("decode export json response: %v", err)
	}
	if len(exportedJSON) < 2 {
		t.Fatalf("expected json export to include events, got %d", len(exportedJSON))
	}

	exportCSVReq := httptest.NewRequest(http.MethodGet, "/cases/"+caseID.String()+"/events/export?format=csv", nil)
	exportCSVReq.Header.Set("Authorization", "Bearer "+login.Token)
	exportCSVW := httptest.NewRecorder()
	router.ServeHTTP(exportCSVW, exportCSVReq)
	if exportCSVW.Code != http.StatusOK {
		t.Fatalf("export csv endpoint status=%d body=%s", exportCSVW.Code, exportCSVW.Body.String())
	}
	reader := csv.NewReader(bytes.NewReader(exportCSVW.Body.Bytes()))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("decode export csv response: %v", err)
	}
	if len(rows) < 3 {
		t.Fatalf("expected csv export rows >= 3, got %d", len(rows))
	}
}

func seedAuditCase(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, caseTypeName string) uuid.UUID {
	t.Helper()
	caseTypeID := seedAdditionalCaseType(t, ctx, db, tenantID, principalID, caseTypeName)
	workflowID, workflowVersion := seedPublishedWorkflow(t, ctx, db, tenantID, principalID, caseTypeName, engine.WorkflowAST{
		Steps: []engine.WorkflowStep{{ID: "bootstrap", Type: "human_task"}},
	})

	var caseID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'open', '{}'::jsonb, $4, $5, $6)
RETURNING id
`, tenantID, caseTypeID, fmt.Sprintf("AUD-%s", uuid.NewString()[:8]), principalID, workflowID, workflowVersion).Scan(&caseID)
	if err != nil {
		t.Fatalf("insert audit test case: %v", err)
	}
	return caseID
}

func appendAuditEvent(ctx context.Context, svc *audit.Service, db *sql.DB, caseID uuid.UUID, eventType, action string, data map[string]any) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = svc.RollbackTx(tx) }()

	var tenantID uuid.UUID
	if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1 FOR UPDATE`, caseID).Scan(&tenantID); err != nil {
		return err
	}
	var actorID uuid.UUID
	if err := tx.QueryRowContext(ctx, `
SELECT id
FROM principals
WHERE tenant_id = $1
ORDER BY created_at
LIMIT 1
`, tenantID).Scan(&actorID); err != nil {
		return err
	}
	if err := svc.RecordCaseEventTx(ctx, tx, caseID, "", eventType, actorID, "human", action, data); err != nil {
		return err
	}
	return svc.CommitTx(tx)
}
