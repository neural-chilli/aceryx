package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	frontendassets "github.com/neural-chilli/aceryx/frontend"
	"github.com/neural-chilli/aceryx/internal/backup"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/expressions"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
	"github.com/neural-chilli/aceryx/internal/observability"
	"github.com/neural-chilli/aceryx/internal/server"
)

func main() {
	observability.SetupLoggerFromEnv(os.Stdout)

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
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
	case "backup":
		if len(os.Args) >= 3 && os.Args[2] == "verify" {
			if err := runBackupVerify(os.Args[3:]); err != nil {
				slog.Error("backup verify failed", "error", err)
				os.Exit(1)
			}
			return
		}
		if err := runBackup(os.Args[2:]); err != nil {
			slog.Error("backup failed", "error", err)
			os.Exit(1)
		}
	case "restore":
		if err := runRestore(os.Args[2:]); err != nil {
			if errors.Is(err, backup.ErrRestoreNeedsConfirm) {
				fmt.Println(err.Error())
				os.Exit(0)
			}
			slog.Error("restore failed", "error", err)
			os.Exit(1)
		}
	case "serve":
		if err := runServe(); err != nil {
			slog.Error("serve failed", "error", err)
			os.Exit(1)
		}
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

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	output := fs.String("output", "", "output backup tar.gz path")
	tenant := fs.String("tenant", "", "tenant id to backup")
	pause := fs.Bool("pause", false, "enable maintenance mode during backup")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return fmt.Errorf("--output is required")
	}

	var tenantID *uuid.UUID
	if *tenant != "" {
		parsed, err := uuid.Parse(*tenant)
		if err != nil {
			return fmt.Errorf("parse --tenant: %w", err)
		}
		tenantID = &parsed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	dbURL := resolveDatabaseURL()
	if dbURL == "" {
		return fmt.Errorf("missing database URL: set ACERYX_DB_URL, ACERYX_DATABASE_URL or DATABASE_URL")
	}
	vaultPath := resolveVaultPath()

	db, err := openDatabaseFromURL(ctx, dbURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	svc := backup.NewService(db, dbURL, vaultPath)
	meta, err := svc.Backup(ctx, backup.BackupOptions{OutputPath: *output, TenantID: tenantID, Pause: *pause})
	if err != nil {
		return err
	}

	fmt.Printf("backup complete\n")
	fmt.Printf("output: %s\n", *output)
	fmt.Printf("created_at: %s\n", meta.CreatedAt.Format(time.RFC3339))
	fmt.Printf("schema_version: %d\n", meta.SchemaVersion)
	fmt.Printf("cases: %d\n", meta.CaseCount)
	fmt.Printf("documents: %d\n", meta.DocumentCount)
	fmt.Printf("size_bytes: %d\n", meta.SizeBytes)
	return nil
}

func runBackupVerify(args []string) error {
	fs := flag.NewFlagSet("backup verify", flag.ContinueOnError)
	input := fs.String("input", "", "backup tar.gz to verify")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return fmt.Errorf("--input is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	svc := backup.NewService(nil, "", resolveVaultPath())
	result, err := svc.Verify(ctx, *input)
	if err != nil {
		return err
	}

	fmt.Printf("Backup: %s\n", *input)
	fmt.Printf("Created: %s\n", result.Metadata.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Aceryx version: %s\n", result.Metadata.AceryxVersion)
	fmt.Printf("Schema version: %d\n", result.Metadata.SchemaVersion)
	fmt.Printf("Cases: %d\n", result.Metadata.CaseCount)
	fmt.Printf("Documents: %d\n", result.Metadata.DocumentCount)
	fmt.Printf("Postgres dump: valid\n")
	fmt.Printf("Vault archive: valid (%d files, %d bytes)\n", result.VaultFileCount, result.VaultSizeBytes)
	fmt.Printf("Total size: %d\n", result.Metadata.SizeBytes)
	fmt.Printf("Status: %s\n", result.Status)
	return nil
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	input := fs.String("input", "", "input backup tar.gz")
	targetDB := fs.String("target-db", "", "override target database connection string")
	confirm := fs.Bool("confirm", false, "confirm destructive restore")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return fmt.Errorf("--input is required")
	}

	dbURL := *targetDB
	if dbURL == "" {
		dbURL = resolveDatabaseURL()
	}
	if dbURL == "" {
		return fmt.Errorf("missing target database URL: set --target-db or ACERYX_DB_URL/ACERYX_DATABASE_URL/DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	db, err := openDatabaseFromURL(ctx, dbURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	svc := backup.NewService(db, dbURL, resolveVaultPath())
	result, err := svc.Restore(ctx, backup.RestoreOptions{
		InputPath:   *input,
		TargetDBURL: *targetDB,
		Confirm:     *confirm,
	})
	if err != nil {
		return err
	}

	fmt.Println("Restore complete.")
	fmt.Printf("Cases: %d\n", result.CasesCount)
	fmt.Printf("Documents: %d\n", result.DocumentsCount)
	if result.MigrationsApplied {
		fmt.Printf("Schema version: %d (migrated from %d)\n", result.SchemaVersion, result.MigratedFrom)
	} else {
		fmt.Printf("Schema version: %d\n", result.SchemaVersion)
	}
	fmt.Printf("Vault files verified: %d/%d\n", result.VaultFilesVerified, result.VaultFilesSampled)
	return nil
}

func runServe() error {
	serverCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	dbCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := openDatabase(dbCtx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	evaluator := expressions.NewEvaluator()
	eng := engine.New(db, evaluator, engine.Config{})
	handler := server.NewHandlerWithContext(serverCtx, db, eng, frontendassets.DistFS())
	go eng.StartSLAMonitor(serverCtx)

	addr := os.Getenv("ACERYX_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		<-serverCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("starting server", "addr", addr)
	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if waitErr := waitForEngineDrain(eng, 20*time.Second); waitErr != nil {
		return waitErr
	}
	return nil
}

func waitForEngineDrain(eng *engine.Engine, timeout time.Duration) error {
	if eng == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		eng.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timed out waiting for engine workers to drain")
	}
}

func openDatabase(ctx context.Context) (*sql.DB, error) {
	databaseURL := resolveDatabaseURL()
	if databaseURL == "" {
		return nil, fmt.Errorf("missing database URL: set ACERYX_DB_URL, ACERYX_DATABASE_URL or DATABASE_URL")
	}
	return openDatabaseFromURL(ctx, databaseURL)
}

func openDatabaseFromURL(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	configureDBPool(db)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

func configureDBPool(db *sql.DB) {
	maxOpen := envInt("ACERYX_DB_MAX_OPEN_CONNS", 50)
	maxIdle := envInt("ACERYX_DB_MAX_IDLE_CONNS", 25)
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(envDuration("ACERYX_DB_CONN_MAX_LIFETIME", time.Hour))
	db.SetConnMaxIdleTime(envDuration("ACERYX_DB_CONN_MAX_IDLE_TIME", 15*time.Minute))
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func resolveDatabaseURL() string {
	if value := os.Getenv("ACERYX_DB_URL"); value != "" {
		return value
	}
	if value := os.Getenv("ACERYX_DATABASE_URL"); value != "" {
		return value
	}
	return os.Getenv("DATABASE_URL")
}

func resolveVaultPath() string {
	if value := os.Getenv("ACERYX_VAULT_PATH"); value != "" {
		return value
	}
	if value := os.Getenv("ACERYX_VAULT_ROOT"); value != "" {
		return value
	}
	return "./data/vault"
}

func printUsage() {
	fmt.Println("aceryx - case orchestration engine")
	fmt.Println("usage: aceryx [serve|migrate|seed|backup|restore|version]")
	fmt.Println("backup usage: aceryx backup --output /path/to/backup.tar.gz [--tenant <tenant_id>] [--pause]")
	fmt.Println("backup verify usage: aceryx backup verify --input /path/to/backup.tar.gz")
	fmt.Println("restore usage: aceryx restore --input /path/to/backup.tar.gz [--target-db <connection_string>] --confirm")
}
