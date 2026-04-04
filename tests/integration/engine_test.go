package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/expressions"
	"github.com/neural-chilli/aceryx/internal/observability"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestEngineIntegration_FullWorkflowExecution(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{ID: "s1", Type: "rule"},
		{ID: "s2", Type: "rule", DependsOn: []string{"s1"}},
	}}
	caseID := seedEngineCase(t, ctx, db, ast)
	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	en.RegisterExecutor("rule", engine.NewMockExecutor(map[string][]engine.MockExecution{
		"s1": {{Result: &engine.StepResult{Output: json.RawMessage(`{"ok":true}`)}}},
		"s2": {{Result: &engine.StepResult{Output: json.RawMessage(`{"ok":true}`)}}},
	}))

	if err := en.EvaluateDAG(ctx, caseID); err != nil {
		t.Fatalf("evaluate dag: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "s1", engine.StateCompleted)
	waitForStepState(t, ctx, db, caseID, "s2", engine.StateCompleted)
}

func TestEngineIntegration_ConcurrentDAGEvaluationActivatesOnce(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{ID: "a", Type: "rule"},
		{ID: "b", Type: "rule"},
		{ID: "join", Type: "rule", DependsOn: []string{"a", "b"}, Join: "all"},
	}}
	caseID := seedEngineCase(t, ctx, db, ast)
	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	en.RegisterExecutor("rule", engine.NewMockExecutor(map[string][]engine.MockExecution{
		"join": {{Result: &engine.StepResult{}}},
	}))

	mustExec(t, ctx, db, `UPDATE case_steps SET state='completed', result='{}'::jsonb WHERE case_id=$1 AND step_id='a'`, caseID)
	mustExec(t, ctx, db, `UPDATE case_steps SET state='completed', result='{}'::jsonb WHERE case_id=$1 AND step_id='b'`, caseID)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = en.EvaluateDAG(ctx, caseID)
		}()
	}
	wg.Wait()

	waitForStepState(t, ctx, db, caseID, "join", engine.StateCompleted)
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM case_steps WHERE case_id=$1 AND step_id='join' AND state='completed'`, caseID).Scan(&count); err != nil {
		t.Fatalf("count completed join: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected join to complete exactly once, got %d", count)
	}
}

func TestEngineIntegration_OptimisticLocking(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "x", Type: "rule"}}}
	caseID := seedEngineCase(t, ctx, db, ast)
	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})

	var version int
	if err := db.QueryRowContext(ctx, `SELECT version FROM cases WHERE id=$1`, caseID).Scan(&version); err != nil {
		t.Fatalf("load case version: %v", err)
	}

	var conflicts int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := en.UpdateCaseData(ctx, caseID, version, json.RawMessage(fmt.Sprintf(`{"writer":%d}`, i)))
			if errors.Is(err, engine.ErrCaseDataConflict) {
				atomic.AddInt32(&conflicts, 1)
			}
		}(i)
	}
	wg.Wait()
	if conflicts != 1 {
		t.Fatalf("expected one optimistic lock conflict, got %d", conflicts)
	}
}

func TestEngineIntegration_RecoveryScan(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{ID: "recover_me", Type: "integration", Metadata: map[string]interface{}{"idempotent": true}},
	}}
	caseID := seedEngineCase(t, ctx, db, ast)
	mustExec(t, ctx, db, `UPDATE case_steps SET state='active' WHERE case_id=$1 AND step_id='recover_me'`, caseID)

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	en.RegisterExecutor("integration", engine.NewMockExecutor(map[string][]engine.MockExecution{
		"recover_me": {{Result: &engine.StepResult{Output: json.RawMessage(`{"recovered":true}`)}}},
	}))

	if err := en.Recover(ctx); err != nil {
		t.Fatalf("recover: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "recover_me", engine.StateCompleted)
}

func TestEngineIntegration_CancellationStopsProgress(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{ID: "inflight", Type: "integration"},
		{ID: "downstream", Type: "rule", DependsOn: []string{"inflight"}},
	}}
	caseID := seedEngineCase(t, ctx, db, ast)
	mustExec(t, ctx, db, `UPDATE case_steps SET state='active' WHERE case_id=$1 AND step_id='inflight'`, caseID)

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	if err := en.CancelCase(ctx, caseID, uuid.New(), "user requested"); err != nil {
		t.Fatalf("cancel case: %v", err)
	}
	if err := en.CompleteStep(ctx, caseID, "inflight", &engine.StepResult{Output: json.RawMessage(`{"done":true}`)}); err != nil {
		t.Fatalf("complete inflight step after cancel: %v", err)
	}

	var state string
	waitForCondition(t, 2*time.Second, 20*time.Millisecond, func() bool {
		if err := db.QueryRowContext(ctx, `SELECT state FROM case_steps WHERE case_id=$1 AND step_id='downstream'`, caseID).Scan(&state); err != nil {
			return false
		}
		return state == engine.StateSkipped || state == engine.StatePending
	}, "downstream step did not settle after cancellation")
	if state != engine.StateSkipped && state != engine.StatePending {
		t.Fatalf("expected downstream not to advance after cancellation, got %s", state)
	}
}

func TestEngineIntegration_ErrorPolicyRetriesAndExhausts(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{
			ID:   "unstable",
			Type: "integration",
			ErrorPolicy: engine.ErrorPolicy{
				MaxAttempts: 2,
				Backoff:     "none",
				OnExhausted: "skip",
			},
		},
	}}
	caseID := seedEngineCase(t, ctx, db, ast)
	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	en.RegisterExecutor("integration", engine.NewMockExecutor(map[string][]engine.MockExecution{
		"unstable": {
			{Err: errors.New("attempt1")},
			{Err: errors.New("attempt2")},
		},
	}))

	if err := en.EvaluateDAG(ctx, caseID); err != nil {
		t.Fatalf("evaluate dag: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "unstable", engine.StateSkipped)
}

func TestEngineIntegration_SLAEscalation(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "sla", Type: "human_task"}}}
	caseID := seedEngineCase(t, ctx, db, ast)
	mustExec(t, ctx, db, `
UPDATE case_steps
SET state='active', sla_deadline=now() - interval '1 minute'
WHERE case_id=$1 AND step_id='sla'
`, caseID)

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{SLAInterval: 100 * time.Millisecond})
	var escalations int32
	en.SetEscalationCallback(func(_ context.Context, _ engine.OverdueTask) error {
		atomic.AddInt32(&escalations, 1)
		return nil
	})

	_, err := en.CheckOverdueTasksForTest(ctx)
	if err != nil {
		t.Fatalf("check overdue tasks: %v", err)
	}
	if atomic.LoadInt32(&escalations) == 0 {
		t.Fatal("expected escalation callback to fire")
	}
}

func TestEngineIntegration_SLACancelledCaseDoesNotEscalate(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "sla", Type: "human_task"}}}
	caseID := seedEngineCase(t, ctx, db, ast)
	mustExec(t, ctx, db, `
UPDATE case_steps
SET state='active', sla_deadline=now() - interval '1 minute'
WHERE case_id=$1 AND step_id='sla'
`, caseID)
	mustExec(t, ctx, db, `UPDATE cases SET status='cancelled' WHERE id=$1`, caseID)

	en := engine.New(db, expressions.NewEvaluator(), engine.Config{SLAInterval: 100 * time.Millisecond})
	var escalations int32
	en.SetEscalationCallback(func(_ context.Context, _ engine.OverdueTask) error {
		atomic.AddInt32(&escalations, 1)
		return nil
	})

	_, err := en.CheckOverdueTasksForTest(ctx)
	if err != nil {
		t.Fatalf("check overdue tasks: %v", err)
	}
	if got := atomic.LoadInt32(&escalations); got != 0 {
		t.Fatalf("expected no escalation callback for cancelled case, got %d", got)
	}

	var breaches int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM case_events WHERE case_id = $1 AND event_type = 'system' AND action = 'sla_breach'`, caseID).Scan(&breaches); err != nil {
		t.Fatalf("count sla breach events: %v", err)
	}
	if breaches != 0 {
		t.Fatalf("expected no sla_breach event for cancelled case, got %d", breaches)
	}
}

func TestEngineIntegration_DAGEvaluationMetricIncrements(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "s1", Type: "rule"}}}
	caseID := seedEngineCase(t, ctx, db, ast)
	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	en.RegisterExecutor("rule", engine.NewMockExecutor(map[string][]engine.MockExecution{
		"s1": {{Result: &engine.StepResult{Output: json.RawMessage(`{"ok":true}`)}}},
	}))

	var tenantID uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1`, caseID).Scan(&tenantID); err != nil {
		t.Fatalf("load tenant id: %v", err)
	}
	before := testutil.ToFloat64(observability.DAGEvaluationsTotal.WithLabelValues(tenantID.String()))
	if err := en.EvaluateDAG(ctx, caseID); err != nil {
		t.Fatalf("evaluate dag: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "s1", engine.StateCompleted)
	after := testutil.ToFloat64(observability.DAGEvaluationsTotal.WithLabelValues(tenantID.String()))
	if after <= before {
		t.Fatalf("expected dag metric to increase, before=%f after=%f", before, after)
	}
}

func seedEngineCase(t *testing.T, ctx context.Context, db *sql.DB, ast engine.WorkflowAST) uuid.UUID {
	t.Helper()

	sfx := uuid.NewString()[:8]
	var tenantID, principalID, caseTypeID, workflowID string

	err := db.QueryRowContext(ctx, `
INSERT INTO tenants (name, slug, branding, terminology, settings)
VALUES ($1, $2, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb)
RETURNING id
`, "Tenant-"+sfx, "tenant-"+sfx).Scan(&tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	err = db.QueryRowContext(ctx, `
INSERT INTO principals (tenant_id, type, name, email, status)
VALUES ($1, 'human', $2, $3, 'active')
RETURNING id
`, tenantID, "Principal-"+sfx, "principal-"+sfx+"@example.com").Scan(&principalID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}
	err = db.QueryRowContext(ctx, `
INSERT INTO case_types (tenant_id, name, version, schema, status, created_by)
VALUES ($1, $2, 1, '{}'::jsonb, 'active', $3)
RETURNING id
`, tenantID, "CaseType-"+sfx, principalID).Scan(&caseTypeID)
	if err != nil {
		t.Fatalf("insert case type: %v", err)
	}
	err = db.QueryRowContext(ctx, `
INSERT INTO workflows (tenant_id, name, case_type, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id
`, tenantID, "Workflow-"+sfx, "CaseType-"+sfx, principalID).Scan(&workflowID)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	astJSON, err := json.Marshal(ast)
	if err != nil {
		t.Fatalf("marshal ast: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workflow_versions (workflow_id, version, status, ast, yaml_source, created_by)
VALUES ($1, 1, 'published', $2::jsonb, '', $3)
`, workflowID, string(astJSON), principalID); err != nil {
		t.Fatalf("insert workflow version: %v", err)
	}

	var caseID uuid.UUID
	err = db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'open', '{}'::jsonb, $4, $5, 1)
RETURNING id
`, tenantID, caseTypeID, "CASE-"+sfx, principalID, workflowID).Scan(&caseID)
	if err != nil {
		t.Fatalf("insert case: %v", err)
	}

	for _, step := range ast.Steps {
		if _, err := db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, events, retry_count)
VALUES ($1, $2, 'pending', '[]'::jsonb, 0)
`, caseID, step.ID); err != nil {
			t.Fatalf("insert case step %s: %v", step.ID, err)
		}
	}
	return caseID
}

func waitForStepState(t *testing.T, ctx context.Context, db *sql.DB, caseID uuid.UUID, stepID, wanted string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var state string
		if err := db.QueryRowContext(ctx, `SELECT state FROM case_steps WHERE case_id=$1 AND step_id=$2`, caseID, stepID).Scan(&state); err == nil {
			if state == wanted {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("step %s did not reach state %s before timeout", stepID, wanted)
}

func mustExec(t *testing.T, ctx context.Context, db *sql.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
}
