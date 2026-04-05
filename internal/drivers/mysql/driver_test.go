package mysql

import (
	"context"
	"testing"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

func TestMySQLDriverMetadataAndConnect(t *testing.T) {
	d := New()
	if d.ID() != "mysql" {
		t.Fatalf("unexpected id %q", d.ID())
	}
	db, err := d.Connect(context.Background(), drivers.DBConfig{
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "aceryx",
		User:     "root",
		Password: "root",
	})
	if err != nil {
		t.Fatalf("connect should build client without immediate dial: %v", err)
	}
	_ = d.Close(db)
}
