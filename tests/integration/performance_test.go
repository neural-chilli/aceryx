package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
)

func TestPerformance_ConcurrentCaseCreationAndList(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	fix := newFixtures(t, ctx, db).
		WithTenant("perf-load").
		WithCaseType("loan_perf", testCaseSchema()).
		WithPublishedWorkflow(engine.WorkflowAST{Steps: []engine.WorkflowStep{{ID: "intake", Type: "human_task"}}}).
		Build()

	svc := cases.NewCaseService(db, nil)
	const total = 24
	var wg sync.WaitGroup
	errs := make(chan error, total)

	for i := 0; i < total; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, validation, err := svc.CreateCase(ctx, fix.TenantID, fix.PrincipalID, cases.CreateCaseRequest{
				CaseType: fix.CaseType,
				Priority: (i % 5) + 1,
				Data: map[string]interface{}{
					"applicant": map[string]interface{}{
						"company_name":        fmt.Sprintf("Perf Co %d", i),
						"registration_number": fmt.Sprintf("%08d", i+10000000),
					},
					"loan": map[string]interface{}{
						"amount": 10000.0 + float64(i*250),
					},
					"decision": "pending",
				},
			})
			if err != nil {
				errs <- err
				return
			}
			if len(validation) > 0 {
				errs <- fmt.Errorf("validation errors: %+v", validation)
				return
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create failed: %v", err)
		}
	}

	rows, err := svc.ListCases(ctx, fix.TenantID, cases.ListCasesFilter{PerPage: total + 5, Page: 1, SortBy: "created_at", SortDir: "desc"})
	if err != nil {
		t.Fatalf("list cases after concurrent create: %v", err)
	}
	if len(rows) < total {
		t.Fatalf("expected at least %d rows, got %d", total, len(rows))
	}
}
