package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPostgresDriverConnectPingQuery(t *testing.T) {
	t.Parallel()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "aceryx",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := ctr.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("mapped port: %v", err)
	}

	d := New()
	db, err := d.Connect(ctx, drivers.DBConfig{
		Host:     host,
		Port:     port.Int(),
		Database: "aceryx",
		User:     "postgres",
		Password: "postgres",
		SSLMode:  "disable",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = d.Close(db) }()
	if err := d.Ping(ctx, db); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS t (id SERIAL PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO t(name) VALUES($1)`, "hello"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var got string
	if err := db.QueryRowContext(ctx, `SELECT name FROM t LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected hello, got %s", fmt.Sprintf("%q", got))
	}
}
