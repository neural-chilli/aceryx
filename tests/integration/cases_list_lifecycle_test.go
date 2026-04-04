package integration

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
)

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
