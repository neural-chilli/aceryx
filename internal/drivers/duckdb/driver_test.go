package duckdb

import (
	"context"
	"testing"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

func TestDuckDBDriverConnectQuery(t *testing.T) {
	d := New()
	db, err := d.Connect(context.Background(), drivers.DBConfig{Database: ":memory:"})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = d.Close(db) }()
	if err := d.Ping(context.Background(), db); err != nil {
		t.Fatalf("ping: %v", err)
	}
	var one int
	if err := db.QueryRow(`SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("query: %v", err)
	}
	if one != 1 {
		t.Fatalf("expected 1, got %d", one)
	}
}
