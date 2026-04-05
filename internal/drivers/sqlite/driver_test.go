package sqlite

import (
	"context"
	"testing"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

func TestSQLiteDriverConnectQueryWAL(t *testing.T) {
	d := New()
	db, err := d.Connect(context.Background(), drivers.DBConfig{Database: ":memory:"})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = d.Close(db) }()
	if err := d.Ping(context.Background(), db); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t(name) VALUES ('a')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM t`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row, got %d", n)
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("pragma journal_mode: %v", err)
	}
	if mode == "" {
		t.Fatal("expected journal mode value")
	}
}
