package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func main() {
	observability.SetupLoggerFromEnv(os.Stdout)

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "version":
		fmt.Println("aceryx v0.0.1-dev")
	case "migrate":
		if err := runMigrate(); err != nil {
			slog.Error("migrate failed", "error", err)
			os.Exit(1)
		}
	case "seed":
		if err := runSeed(); err != nil {
			slog.Error("seed failed", "error", err)
			os.Exit(1)
		}
	case "serve":
		fmt.Println("aceryx - case orchestration engine")
	default:
		printUsage()
	}
}

func runMigrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	runner := internalmigrations.NewRunner(db)
	if err := runner.Apply(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	fmt.Println("migrations applied")
	return nil
}

func runSeed() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	runner := internalmigrations.NewRunner(db)
	if err := runner.Apply(ctx); err != nil {
		return fmt.Errorf("apply migrations before seed: %w", err)
	}

	if err := internalmigrations.SeedDefaultData(ctx, db); err != nil {
		return fmt.Errorf("seed default data: %w", err)
	}

	fmt.Println("default seed applied")
	return nil
}

func openDatabase(ctx context.Context) (*sql.DB, error) {
	databaseURL := os.Getenv("ACERYX_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return nil, fmt.Errorf("missing database URL: set ACERYX_DATABASE_URL or DATABASE_URL")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

func printUsage() {
	fmt.Println("aceryx - case orchestration engine")
	fmt.Println("usage: aceryx [serve|migrate|seed|version]")
}
