package drivers

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Require(_ context.Context, _ string) error { return nil }

type denyAllAuthorizer struct{}

func (denyAllAuthorizer) Require(_ context.Context, _ string) error { return errors.New("denied") }

func setupSQLiteForQueryTests(t *testing.T) (*DriverRegistry, *PoolManager) {
	t.Helper()
	reg := NewDriverRegistry()
	reg.RegisterDB(&testSQLiteDriver{})
	pm := NewPoolManager()
	qe := NewQueryExecutor(reg, pm, allowAllAuthorizer{})
	_, err := qe.Execute(context.Background(), QueryRequest{
		TenantID: "t1",
		DriverID: "sqlite",
		Config:   DBConfig{Database: ":memory:"},
		Query:    "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)",
		ReadOnly: false,
	})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return reg, pm
}

func TestQueryExecutorParameterizedAndInjectionSafe(t *testing.T) {
	reg, pm := setupSQLiteForQueryTests(t)
	qe := NewQueryExecutor(reg, pm, allowAllAuthorizer{})

	_, err := qe.Execute(context.Background(), QueryRequest{
		TenantID: "t1",
		DriverID: "sqlite",
		Config:   DBConfig{Database: ":memory:"},
		Query:    "INSERT INTO users(name) VALUES(?)",
		Params:   []any{"alice"},
		ReadOnly: false,
	})
	if err != nil {
		t.Fatalf("insert alice: %v", err)
	}
	malicious := "'; DROP TABLE users; --"
	_, err = qe.Execute(context.Background(), QueryRequest{
		TenantID: "t1",
		DriverID: "sqlite",
		Config:   DBConfig{Database: ":memory:"},
		Query:    "INSERT INTO users(name) VALUES(?)",
		Params:   []any{malicious},
		ReadOnly: false,
	})
	if err != nil {
		t.Fatalf("insert malicious literal: %v", err)
	}

	res, err := qe.Execute(context.Background(), QueryRequest{
		TenantID: "t1",
		DriverID: "sqlite",
		Config:   DBConfig{Database: ":memory:"},
		Query:    "SELECT name FROM users WHERE name = ?",
		Params:   []any{malicious},
		ReadOnly: true,
	})
	if err != nil {
		t.Fatalf("select malicious literal: %v", err)
	}
	if res.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", res.RowCount)
	}

	_, err = qe.Execute(context.Background(), QueryRequest{
		TenantID: "t1",
		DriverID: "sqlite",
		Config:   DBConfig{Database: ":memory:"},
		Query:    "SELECT count(*) FROM users",
		ReadOnly: true,
	})
	if err != nil {
		t.Fatalf("table should still exist after injection attempt: %v", err)
	}
}

func TestQueryExecutorReadOnlyWriteDenied(t *testing.T) {
	reg, pm := setupSQLiteForQueryTests(t)
	qe := NewQueryExecutor(reg, pm, denyAllAuthorizer{})
	_, err := qe.Execute(context.Background(), QueryRequest{
		TenantID: "t1",
		DriverID: "sqlite",
		Config:   DBConfig{Database: ":memory:"},
		Query:    "INSERT INTO users(name) VALUES(?)",
		Params:   []any{"bob"},
		ReadOnly: false,
	})
	if err == nil {
		t.Fatal("expected write permission error")
	}
}

func TestQueryExecutorRowLimitAndTruncation(t *testing.T) {
	reg, pm := setupSQLiteForQueryTests(t)
	qe := NewQueryExecutor(reg, pm, allowAllAuthorizer{})
	for i := 0; i < 5; i++ {
		_, err := qe.Execute(context.Background(), QueryRequest{
			TenantID: "t1",
			DriverID: "sqlite",
			Config:   DBConfig{Database: ":memory:"},
			Query:    "INSERT INTO users(name) VALUES(?)",
			Params:   []any{time.Now().String()},
			ReadOnly: false,
		})
		if err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}

	res, err := qe.Execute(context.Background(), QueryRequest{
		TenantID: "t1",
		DriverID: "sqlite",
		Config:   DBConfig{Database: ":memory:"},
		Query:    "SELECT id, name FROM users ORDER BY id",
		ReadOnly: true,
		RowLimit: 3,
	})
	if err != nil {
		t.Fatalf("select with row limit: %v", err)
	}
	if res.RowCount != 3 {
		t.Fatalf("expected 3 rows, got %d", res.RowCount)
	}
	if !res.Truncated {
		t.Fatal("expected truncated=true")
	}
}

func TestQueryExecutorTimeout(t *testing.T) {
	reg := NewDriverRegistry()
	reg.RegisterDB(&slowPingDriver{})
	pm := NewPoolManager()
	qe := NewQueryExecutor(reg, pm, allowAllAuthorizer{})
	_, err := qe.Execute(context.Background(), QueryRequest{
		TenantID: "t1",
		DriverID: "slow",
		Config:   DBConfig{TimeoutSecs: 30},
		Query:    "SELECT 1",
		ReadOnly: true,
		Timeout:  10 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

type slowPingDriver struct{}

func (d *slowPingDriver) ID() string          { return "slow" }
func (d *slowPingDriver) DisplayName() string { return "Slow" }
func (d *slowPingDriver) Connect(_ context.Context, _ DBConfig) (*sql.DB, error) {
	return sql.Open("sqlite", ":memory:")
}
func (d *slowPingDriver) Ping(ctx context.Context, _ *sql.DB) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}
func (d *slowPingDriver) Close(db *sql.DB) error { return db.Close() }

type testSQLiteDriver struct{}

func (d *testSQLiteDriver) ID() string          { return "sqlite" }
func (d *testSQLiteDriver) DisplayName() string { return "SQLite" }
func (d *testSQLiteDriver) Connect(_ context.Context, cfg DBConfig) (*sql.DB, error) {
	path := cfg.Database
	if path == "" {
		path = ":memory:"
	}
	return sql.Open("sqlite", path)
}
func (d *testSQLiteDriver) Ping(_ context.Context, db *sql.DB) error { return db.Ping() }
func (d *testSQLiteDriver) Close(db *sql.DB) error                   { return db.Close() }
