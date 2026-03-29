package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/expressions"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

type notifySpy struct {
	mu       sync.Mutex
	userMsgs []map[string]any
	roleMsgs []map[string]any
}

func (n *notifySpy) NotifyUser(_ context.Context, _ uuid.UUID, payload map[string]any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.userMsgs = append(n.userMsgs, payload)
	return nil
}

func (n *notifySpy) NotifyRole(_ context.Context, _ uuid.UUID, _ string, payload map[string]any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.roleMsgs = append(n.roleMsgs, payload)
	return nil
}

func TestTasksIntegration_HumanTaskFlow(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, ownerID := seedTenantAndPrincipal(t, ctx, db, "t004-flow")
	workerID := seedPrincipal(t, ctx, db, tenantID, "worker", "worker@flow.local")
	mustSeedRoleMembership(t, ctx, db, tenantID, workerID, "case_worker")

	stepCfg := map[string]any{
		"assign_to_role": "case_worker",
		"sla_hours":      2,
		"form":           "review_form",
		"form_schema": map[string]any{
			"fields": []map[string]any{
				{"id": "decision_notes", "type": "string", "required": true, "bind": "decision.notes"},
			},
		},
		"escalation": map[string]any{"action": "notify", "to_role": "case_worker"},
	}
	cfgRaw, _ := json.Marshal(stepCfg)
	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{
			ID:        "review",
			Type:      "human_task",
			Config:    cfgRaw,
			Outcomes:  map[string][]string{"approve": []string{"archive"}, "reject": []string{}},
			DependsOn: nil,
		},
		{ID: "archive", Type: "rule", DependsOn: []string{"review"}},
	}}
	caseID := seedTaskCase(t, ctx, db, tenantID, ownerID, "flow_case", ast)

	notify := &notifySpy{}
	en := engine.New(db, expressions.NewEvaluator(), engine.Config{})
	taskSvc := tasks.NewTaskService(db, en, notify)
	en.RegisterExecutor("human_task", tasks.NewHumanTaskExecutor(taskSvc))
	en.RegisterExecutor("rule", engine.NewMockExecutor(map[string][]engine.MockExecution{
		"archive": {{Result: &engine.StepResult{Output: json.RawMessage(`{"archived":true}`)}}},
	}))

	if err := en.EvaluateDAG(ctx, caseID); err != nil {
		t.Fatalf("evaluate dag: %v", err)
	}
	waitForStepState(t, ctx, db, caseID, "review", engine.StateActive)

	inbox, err := taskSvc.Inbox(ctx, tenantID, workerID)
	if err != nil {
		t.Fatalf("inbox query: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected one claimable inbox task, got %d", len(inbox))
	}
	if inbox[0].SLAStatus == "" {
		t.Fatal("expected SLA status to be populated")
	}

	if err := taskSvc.ClaimTask(ctx, tenantID, workerID, caseID, "review"); err != nil {
		t.Fatalf("claim task: %v", err)
	}
	if err := taskSvc.SaveDraft(ctx, tenantID, workerID, caseID, "review", tasks.DraftRequest{Data: map[string]any{"decision_notes": "pending final review"}}); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	if err := taskSvc.CompleteTask(ctx, tenantID, workerID, caseID, "review", tasks.CompleteTaskRequest{
		Outcome: "approve",
		Data:    map[string]any{"decision_notes": "approved"},
	}); err != nil {
		t.Fatalf("complete task: %v", err)
	}

	waitForStepState(t, ctx, db, caseID, "archive", engine.StateCompleted)

	var (
		state     string
		resultRaw []byte
		draftRaw  []byte
	)
	if err := db.QueryRowContext(ctx, `SELECT state, result, draft_data FROM case_steps WHERE case_id=$1 AND step_id='review'`, caseID).Scan(&state, &resultRaw, &draftRaw); err != nil {
		t.Fatalf("load review step: %v", err)
	}
	if state != engine.StateCompleted {
		t.Fatalf("expected review completed, got %s", state)
	}
	var result map[string]any
	_ = json.Unmarshal(resultRaw, &result)
	if got, _ := result["outcome"].(string); got != "approve" {
		t.Fatalf("expected outcome approve in result, got %#v", result["outcome"])
	}
	if len(draftRaw) != 0 {
		t.Fatalf("expected draft data cleared, got %s", string(draftRaw))
	}

	var eventCount int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM case_events
WHERE case_id=$1 AND event_type IN ('task_created','task_claimed','task_completed')
`, caseID).Scan(&eventCount); err != nil {
		t.Fatalf("count task audit events: %v", err)
	}
	if eventCount < 3 {
		t.Fatalf("expected at least 3 task audit events, got %d", eventCount)
	}

	if len(notify.roleMsgs) == 0 {
		t.Fatal("expected role notifications to be sent")
	}
}

func TestTasksIntegration_ClaimAtomicAndAlreadyClaimed(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, ownerID := seedTenantAndPrincipal(t, ctx, db, "t004-claim")
	p1 := seedPrincipal(t, ctx, db, tenantID, "worker-1", "worker1@claim.local")
	p2 := seedPrincipal(t, ctx, db, tenantID, "worker-2", "worker2@claim.local")
	mustSeedRoleMembership(t, ctx, db, tenantID, p1, "case_worker")
	mustSeedRoleMembership(t, ctx, db, tenantID, p2, "case_worker")

	cfgRaw, _ := json.Marshal(map[string]any{"assign_to_role": "case_worker"})
	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "review", Type: "human_task", Config: cfgRaw}}}
	caseID := seedTaskCase(t, ctx, db, tenantID, ownerID, "claim_case", ast)

	notify := &notifySpy{}
	taskSvc := tasks.NewTaskService(db, nil, notify)
	if err := taskSvc.CreateTaskFromActivation(ctx, caseID, "review", tasks.AssignmentConfig{AssignToRole: "case_worker"}); err != nil {
		t.Fatalf("activate task for claim test: %v", err)
	}

	var successes int32
	claim := func(userID uuid.UUID) {
		err := taskSvc.ClaimTask(ctx, tenantID, userID, caseID, "review")
		if err == nil {
			atomic.AddInt32(&successes, 1)
			return
		}
		if !errors.Is(err, tasks.ErrAlreadyClaimed) {
			t.Errorf("unexpected claim error: %v", err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				claim(p1)
				return
			}
			claim(p2)
		}(i)
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("expected exactly one successful claim, got %d", successes)
	}

	if err := taskSvc.ClaimTask(ctx, tenantID, p1, caseID, "review"); !errors.Is(err, tasks.ErrAlreadyClaimed) {
		t.Fatalf("expected already claimed error, got %v", err)
	}
}

func TestTasksIntegration_CompletionValidationReassignEscalation(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantID, ownerID := seedTenantAndPrincipal(t, ctx, db, "t004-complete")
	assigneeID := seedPrincipal(t, ctx, db, tenantID, "assignee", "assignee@complete.local")
	otherID := seedPrincipal(t, ctx, db, tenantID, "other", "other@complete.local")
	manager1 := seedPrincipal(t, ctx, db, tenantID, "manager-1", "manager1@complete.local")
	manager2 := seedPrincipal(t, ctx, db, tenantID, "manager-2", "manager2@complete.local")
	mustSeedRoleMembership(t, ctx, db, tenantID, manager1, "manager")
	mustSeedRoleMembership(t, ctx, db, tenantID, manager2, "manager")

	cfgRaw, _ := json.Marshal(map[string]any{
		"assign_to_user": assigneeID.String(),
		"form_schema": map[string]any{
			"fields": []map[string]any{
				{"id": "notes", "type": "string", "required": true},
			},
		},
		"escalation": map[string]any{"action": "reassign", "to_role": "manager"},
	})
	ast := engine.WorkflowAST{Steps: []engine.WorkflowStep{
		{ID: "review", Type: "human_task", Config: cfgRaw, Outcomes: map[string][]string{"approve": []string{}}},
	}}
	caseID := seedTaskCase(t, ctx, db, tenantID, ownerID, "complete_case", ast)

	notify := &notifySpy{}
	taskSvc := tasks.NewTaskService(db, nil, notify)
	if err := taskSvc.CreateTaskFromActivation(ctx, caseID, "review", tasks.AssignmentConfig{
		AssignToUser: assigneeID.String(),
		FormSchema:   tasks.FormSchema{Fields: []tasks.FormField{{ID: "notes", Type: "string", Required: true}}},
		Escalation:   tasks.EscalationConfig{Action: "reassign", ToRole: "manager"},
	}); err != nil {
		t.Fatalf("activate task: %v", err)
	}

	if err := taskSvc.CompleteTask(ctx, tenantID, assigneeID, caseID, "review", tasks.CompleteTaskRequest{
		Outcome: "reject",
		Data:    map[string]any{"notes": "invalid outcome"},
	}); !errors.Is(err, tasks.ErrInvalidOutcome) {
		t.Fatalf("expected invalid outcome error, got %v", err)
	}

	if err := taskSvc.CompleteTask(ctx, tenantID, otherID, caseID, "review", tasks.CompleteTaskRequest{
		Outcome: "approve",
		Data:    map[string]any{"notes": "forbidden"},
	}); !errors.Is(err, tasks.ErrForbidden) {
		t.Fatalf("expected forbidden error for non-assignee, got %v", err)
	}

	if err := taskSvc.SaveDraft(ctx, tenantID, assigneeID, caseID, "review", tasks.DraftRequest{Data: map[string]any{"notes": "draft survives"}}); err != nil {
		t.Fatalf("save draft before reassign: %v", err)
	}
	if err := taskSvc.ReassignTask(ctx, tenantID, ownerID, caseID, "review", tasks.ReassignRequest{AssignTo: manager1, Reason: "load balancing"}); err != nil {
		t.Fatalf("reassign task: %v", err)
	}
	detail, err := taskSvc.GetTask(ctx, tenantID, caseID, "review")
	if err != nil {
		t.Fatalf("get task after reassign: %v", err)
	}
	if detail.AssignedTo == nil || *detail.AssignedTo != manager1 {
		t.Fatalf("expected task assigned to manager1, got %#v", detail.AssignedTo)
	}
	if string(detail.DraftData) == "" || string(detail.DraftData) == "null" {
		t.Fatal("expected draft data retained across reassignment")
	}

	// Make manager1 busier than manager2 to verify least-loaded selection.
	otherCase := seedTaskCase(t, ctx, db, tenantID, ownerID, "load_case", ast)
	if err := taskSvc.CreateTaskFromActivation(ctx, otherCase, "review", tasks.AssignmentConfig{AssignToUser: manager1.String()}); err != nil {
		t.Fatalf("seed load task: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE case_steps SET assigned_to=$2 WHERE case_id=$1 AND step_id='review'`, caseID, manager1); err != nil {
		t.Fatalf("set current task manager1 before escalation: %v", err)
	}
	if err := taskSvc.EscalateTask(ctx, tenantID, caseID, "review", tasks.EscalationConfig{Action: "reassign", ToRole: "manager"}); err != nil {
		t.Fatalf("escalate task: %v", err)
	}
	var escalatedTo uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT assigned_to FROM case_steps WHERE case_id=$1 AND step_id='review'`, caseID).Scan(&escalatedTo); err != nil {
		t.Fatalf("load escalated assignee: %v", err)
	}
	if escalatedTo != manager2 {
		t.Fatalf("expected escalation to least-loaded manager2, got %s", escalatedTo)
	}

	if _, err := db.ExecContext(ctx, `UPDATE case_steps SET state='completed', completed_at=now() WHERE case_id=$1 AND step_id='review'`, caseID); err != nil {
		t.Fatalf("set completed for suppression check: %v", err)
	}
	if err := taskSvc.EscalateTask(ctx, tenantID, caseID, "review", tasks.EscalationConfig{Action: "notify", ToRole: "manager"}); err != nil {
		t.Fatalf("escalate completed task: %v", err)
	}
	var suppressed int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM case_events
WHERE case_id=$1 AND step_id='review' AND event_type='task_escalation_suppressed'
`, caseID).Scan(&suppressed); err != nil {
		t.Fatalf("count suppressed events: %v", err)
	}
	if suppressed == 0 {
		t.Fatal("expected escalation suppression event")
	}

	if err := taskSvc.CompleteTask(ctx, tenantID, manager2, caseID, "review", tasks.CompleteTaskRequest{
		Outcome: "approve",
		Data:    map[string]any{"notes": "already done"},
	}); !errors.Is(err, tasks.ErrAlreadyCompleted) {
		t.Fatalf("expected already completed error, got %v", err)
	}
}

func seedTaskCase(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, caseTypeName string, ast engine.WorkflowAST) uuid.UUID {
	t.Helper()
	caseTypeID := seedAdditionalCaseType(t, ctx, db, tenantID, principalID, caseTypeName)
	workflowID, workflowVersion := seedPublishedWorkflow(t, ctx, db, tenantID, principalID, caseTypeName, ast)

	var caseID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version, priority)
VALUES ($1, $2, $3, 'open', $4::jsonb, $5, $6, $7, 2)
RETURNING id
`, tenantID, caseTypeID, "TSK-"+uuid.NewString()[:8], `{"applicant":{"company_name":"Acme","registration_number":"12345678"},"loan":{"amount":10000},"decision":"pending"}`, principalID, workflowID, workflowVersion).Scan(&caseID)
	if err != nil {
		t.Fatalf("insert task case: %v", err)
	}

	for _, step := range ast.Steps {
		if _, err := db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, events, retry_count, metadata)
VALUES ($1, $2, 'pending', '[]'::jsonb, 0, '{}'::jsonb)
`, caseID, step.ID); err != nil {
			t.Fatalf("insert case step %s: %v", step.ID, err)
		}
	}
	return caseID
}

func seedPrincipal(t *testing.T, ctx context.Context, db *sql.DB, tenantID uuid.UUID, name string, email string) uuid.UUID {
	t.Helper()
	var principalID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO principals (tenant_id, type, name, email, status, created_at)
VALUES ($1, 'human', $2, $3, 'active', $4)
RETURNING id
`, tenantID, name, email, time.Now().UTC()).Scan(&principalID); err != nil {
		t.Fatalf("insert principal %s: %v", name, err)
	}
	return principalID
}

func mustSeedRoleMembership(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, roleName string) uuid.UUID {
	t.Helper()
	var roleID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO roles (tenant_id, name)
VALUES ($1, $2)
ON CONFLICT (tenant_id, name) DO UPDATE SET name = EXCLUDED.name
RETURNING id
`, tenantID, roleName).Scan(&roleID)
	if err != nil {
		t.Fatalf("insert role %s: %v", roleName, err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO principal_roles (principal_id, role_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`, principalID, roleID); err != nil {
		t.Fatalf("insert principal role %s: %v", roleName, err)
	}
	return roleID
}
