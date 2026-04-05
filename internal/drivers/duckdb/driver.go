package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/marcboeker/go-duckdb"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

type Driver struct{}

func New() *Driver { return &Driver{} }

func (d *Driver) ID() string          { return "duckdb" }
func (d *Driver) DisplayName() string { return "DuckDB" }

func (d *Driver) Connect(ctx context.Context, config drivers.DBConfig) (*sql.DB, error) {
	_ = ctx
	path := strings.TrimSpace(config.Database)
	if path == "" {
		path = ":memory:"
	}
	dsn := path
	if config.ReadOnly {
		dsn = dsn + "?access_mode=read_only"
	}
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	max := config.MaxConns
	if max <= 0 {
		max = 4
	}
	db.SetMaxOpenConns(max)
	return db, nil
}

func (d *Driver) Ping(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping duckdb: %w", err)
	}
	return nil
}

func (d *Driver) Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}
