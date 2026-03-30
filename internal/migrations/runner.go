package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	sqlmigrations "github.com/neural-chilli/aceryx/migrations"
)

type migrationFile struct {
	name    string
	version int
}

// Runner applies embedded SQL migrations in version order.
type Runner struct {
	db *sql.DB
}

func NewRunner(db *sql.DB) *Runner {
	return &Runner{db: db}
}

func LatestVersion() (int, error) {
	files, err := migrationFiles()
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, nil
	}
	return files[len(files)-1].version, nil
}

func (r *Runner) Apply(ctx context.Context) error {
	if err := ensureSchemaMigrations(ctx, r.db); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	files, err := migrationFiles()
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}

	applied, err := appliedVersions(ctx, r.db)
	if err != nil {
		return fmt.Errorf("load applied migrations: %w", err)
	}

	for _, mf := range files {
		if applied[mf.version] {
			continue
		}

		sqlBytes, err := fs.ReadFile(sqlmigrations.Files, mf.name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", mf.name, err)
		}

		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration tx for %s: %w", mf.name, err)
		}

		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", mf.name, err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, mf.version,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", mf.name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", mf.name, err)
		}
	}

	return nil
}

func ensureSchemaMigrations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}
	return nil
}

func migrationFiles() ([]migrationFile, error) {
	entries, err := fs.ReadDir(sqlmigrations.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations directory: %w", err)
	}

	files := make([]migrationFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) != ".sql" {
			continue
		}

		base := strings.TrimSuffix(name, ".sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid migration file name %q", name)
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("parse migration version from %q: %w", name, err)
		}

		files = append(files, migrationFile{name: name, version: version})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].version == files[j].version {
			return files[i].name < files[j].name
		}
		return files[i].version < files[j].version
	})

	return files, nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema_migrations version: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema_migrations versions: %w", err)
	}

	return applied, nil
}
