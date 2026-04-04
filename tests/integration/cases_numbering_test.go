package integration

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
)

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
