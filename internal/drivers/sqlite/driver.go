package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/drivers"
	_ "modernc.org/sqlite"
)

type Driver struct{}

func New() *Driver { return &Driver{} }

func (d *Driver) ID() string          { return "sqlite" }
func (d *Driver) DisplayName() string { return "SQLite" }

func (d *Driver) Connect(ctx context.Context, config drivers.DBConfig) (*sql.DB, error) {
	_ = ctx
	path := strings.TrimSpace(config.Database)
	if path == "" {
		path = ":memory:"
	}
	if path != ":memory:" {
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	max := 1
	if config.MaxConns > 0 {
		max = config.MaxConns
	}
	db.SetMaxOpenConns(max)
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable wal: %w", err)
	}
	return db, nil
}

func (d *Driver) Ping(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	return nil
}

func (d *Driver) Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}
