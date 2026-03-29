package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestSchemaMigrationsAndConstraints(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	t.Run("migration applies cleanly", func(t *testing.T) {
		runner := internalmigrations.NewRunner(db)
		if err := runner.Apply(ctx); err != nil {
			t.Fatalf("re-applying migrations failed: %v", err)
		}
	})

	t.Run("schema_migrations tracks applied version", func(t *testing.T) {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&count); err != nil {
			t.Fatalf("query schema_migrations: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected migration version 1 to be tracked once, got %d", count)
		}
	})

	t.Run("unique constraints enforce correctly", func(t *testing.T) {
		tenantID, principalID, caseTypeID, workflowID := insertBaseFixtures(t, ctx, db, "u1")

		caseID := insertCase(t, ctx, db, tenantID, principalID, caseTypeID, workflowID, "LOAN-000001")

		_, err := db.ExecContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'open', '{}'::jsonb, $4, $5, 1)
`, tenantID, caseTypeID, "LOAN-000001", principalID, workflowID)
		assertPGCode(t, err, "23505")

		if _, err := db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state)
VALUES ($1, $2, 'pending')
`, caseID, "collect-documents"); err != nil {
			t.Fatalf("insert first case step: %v", err)
		}

		_, err = db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state)
VALUES ($1, $2, 'pending')
`, caseID, "collect-documents")
		assertPGCode(t, err, "23505")
	})

	t.Run("check constraints reject invalid enum values", func(t *testing.T) {
		tenantID, principalID, caseTypeID, workflowID := insertBaseFixtures(t, ctx, db, "c1")
		caseID := insertCase(t, ctx, db, tenantID, principalID, caseTypeID, workflowID, "LOAN-000002")

		_, err := db.ExecContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'invalid_status', '{}'::jsonb, $4, $5, 1)
`, tenantID, caseTypeID, "LOAN-000003", principalID, workflowID)
		assertPGCode(t, err, "23514")

		_, err = db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state)
VALUES ($1, $2, 'invalid_state')
`, caseID, "bad-state-step")
		assertPGCode(t, err, "23514")

		_, err = db.ExecContext(ctx, `
INSERT INTO case_events (case_id, event_type, actor_id, actor_type, action, prev_event_hash, event_hash)
VALUES ($1, 'step_started', $2, 'invalid_actor', 'start', 'prev-hash', 'event-hash')
`, caseID, principalID)
		assertPGCode(t, err, "23514")
	})

	t.Run("case number generation is atomic under concurrency", func(t *testing.T) {
		tenantID, _, _, _ := insertBaseFixtures(t, ctx, db, "seq1")

		const workers = 10
		results := make(chan int64, workers)
		errCh := make(chan error, workers)

		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				var next int64
				err := db.QueryRowContext(ctx, `
INSERT INTO case_number_sequences (tenant_id, case_type, last_number)
VALUES ($1, $2, 1)
ON CONFLICT (tenant_id, case_type)
DO UPDATE SET last_number = case_number_sequences.last_number + 1
RETURNING last_number
`, tenantID, "loan").Scan(&next)
				if err != nil {
					errCh <- err
					return
				}
				results <- next
			}()
		}

		wg.Wait()
		close(results)
		close(errCh)

		for err := range errCh {
			if err != nil {
				t.Fatalf("case number generation query failed: %v", err)
			}
		}

		vals := make([]int64, 0, workers)
		for v := range results {
			vals = append(vals, v)
		}

		if len(vals) != workers {
			t.Fatalf("expected %d generated sequence values, got %d", workers, len(vals))
		}

		sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
		for i, v := range vals {
			expected := int64(i + 1)
			if v != expected {
				t.Fatalf("expected sequence value %d at index %d, got %d (values=%v)", expected, i, v, vals)
			}
		}
	})
}

func setupPostgresWithMigrations(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "pgvector/pgvector:pg17",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "aceryx",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("resolve container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("resolve container port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://postgres:postgres@%s:%s/aceryx?sslmode=disable", host, port.Port())
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("open db: %v", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		t.Fatalf("ping db: %v", err)
	}

	runner := internalmigrations.NewRunner(db)
	if err := runner.Apply(ctx); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		t.Fatalf("apply migrations: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = container.Terminate(context.Background())
	}
	return db, cleanup
}

func insertBaseFixtures(t *testing.T, ctx context.Context, db *sql.DB, suffix string) (tenantID, principalID, caseTypeID, workflowID string) {
	t.Helper()

	err := db.QueryRowContext(ctx, `
INSERT INTO tenants (name, slug, branding, terminology, settings)
VALUES ($1, $2, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb)
RETURNING id
`, "Tenant "+suffix, "tenant-"+suffix).Scan(&tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	err = db.QueryRowContext(ctx, `
INSERT INTO principals (tenant_id, type, name, email, status)
VALUES ($1, 'human', $2, $3, 'active')
RETURNING id
`, tenantID, "Principal "+suffix, "principal-"+suffix+"@example.com").Scan(&principalID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	err = db.QueryRowContext(ctx, `
INSERT INTO case_types (tenant_id, name, version, schema, status, created_by)
VALUES ($1, $2, 1, '{}'::jsonb, 'active', $3)
RETURNING id
`, tenantID, "Loan "+suffix, principalID).Scan(&caseTypeID)
	if err != nil {
		t.Fatalf("insert case type: %v", err)
	}

	err = db.QueryRowContext(ctx, `
INSERT INTO workflows (tenant_id, name, case_type, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id
`, tenantID, "Workflow "+suffix, "Loan "+suffix, principalID).Scan(&workflowID)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	return tenantID, principalID, caseTypeID, workflowID
}

func insertCase(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID, caseTypeID, workflowID, caseNumber string) string {
	t.Helper()
	var caseID string
	err := db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'open', '{}'::jsonb, $4, $5, 1)
RETURNING id
`, tenantID, caseTypeID, caseNumber, principalID, workflowID).Scan(&caseID)
	if err != nil {
		t.Fatalf("insert case: %v", err)
	}
	return caseID
}

func assertPGCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected postgres error code %s, got nil", code)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected postgres error with code %s, got: %v", code, err)
	}

	if pgErr.Code != code {
		t.Fatalf("expected postgres error code %s, got %s (%s)", code, pgErr.Code, pgErr.Message)
	}
}
