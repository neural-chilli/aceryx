package drivers

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
)

type countingDriver struct {
	connects atomic.Int64
}

func (d *countingDriver) ID() string          { return "sqlite" }
func (d *countingDriver) DisplayName() string { return "SQLite" }
func (d *countingDriver) Connect(_ context.Context, _ DBConfig) (*sql.DB, error) {
	d.connects.Add(1)
	return sql.Open("sqlite", ":memory:")
}
func (d *countingDriver) Ping(_ context.Context, db *sql.DB) error { return db.Ping() }
func (d *countingDriver) Close(db *sql.DB) error                   { return db.Close() }

func TestPoolManagerReuseAndClose(t *testing.T) {
	pm := NewPoolManager()
	drv := &countingDriver{}
	ctx := context.Background()

	cfg := DBConfig{Database: ":memory:"}
	db1, err := pm.GetOrCreate(ctx, "tenant-a", drv, cfg)
	if err != nil {
		t.Fatalf("GetOrCreate first: %v", err)
	}
	db2, err := pm.GetOrCreate(ctx, "tenant-a", drv, cfg)
	if err != nil {
		t.Fatalf("GetOrCreate second: %v", err)
	}
	if db1 != db2 {
		t.Fatal("expected same pooled db")
	}
	if got := drv.connects.Load(); got != 1 {
		t.Fatalf("expected one connect, got %d", got)
	}

	cfg2 := DBConfig{Database: ":memory:", Host: "changed"}
	db3, err := pm.GetOrCreate(ctx, "tenant-a", drv, cfg2)
	if err != nil {
		t.Fatalf("GetOrCreate changed config: %v", err)
	}
	if db3 == db1 {
		t.Fatal("expected different pool for changed config")
	}
	if got := drv.connects.Load(); got != 2 {
		t.Fatalf("expected two connects, got %d", got)
	}

	if err := pm.Close("tenant-a", "sqlite"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if stats := pm.Stats(); len(stats) != 0 {
		t.Fatalf("expected no pools after close, got %d", len(stats))
	}
}

func TestPoolManagerCloseAll(t *testing.T) {
	pm := NewPoolManager()
	drv := &countingDriver{}
	ctx := context.Background()
	_, _ = pm.GetOrCreate(ctx, "tenant-a", drv, DBConfig{Database: ":memory:"})
	_, _ = pm.GetOrCreate(ctx, "tenant-b", drv, DBConfig{Database: ":memory:", Host: "x"})
	if err := pm.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
	if stats := pm.Stats(); len(stats) != 0 {
		t.Fatalf("expected no pools after close all, got %d", len(stats))
	}
}
