package integration

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/neural-chilli/aceryx/internal/backup"
	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestBackupRestoreRoundTrip(t *testing.T) {
	requirePGTools(t)

	db, dsn, cleanup := setupPostgresWithMigrationsDSN(t)
	defer cleanup()
	ctx := context.Background()

	tenantID, principalID, caseTypeID, workflowID := insertBaseFixtures(t, ctx, db, "backup")
	caseID := insertCase(t, ctx, db, tenantID, principalID, caseTypeID, workflowID, "BKP-001")

	vaultRoot := filepath.Join(t.TempDir(), "vault")
	uri := filepath.Join(tenantID, "2026", "03", "ab", "cd", "doc.txt")
	fullPath := filepath.Join(vaultRoot, uri)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir vault path: %v", err)
	}
	payload := []byte("backup payload")
	if err := os.WriteFile(fullPath, payload, 0o644); err != nil {
		t.Fatalf("write vault file: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO vault_documents (tenant_id, case_id, filename, mime_type, size_bytes, content_hash, storage_uri, uploaded_by)
VALUES ($1, $2, 'doc.txt', 'text/plain', $3, repeat('a',64), $4, $5)
`, tenantID, caseID, len(payload), filepath.ToSlash(uri), principalID); err != nil {
		t.Fatalf("insert vault metadata: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	svc := backup.NewService(db, dsn, vaultRoot)
	meta, err := svc.Backup(ctx, backup.BackupOptions{OutputPath: archivePath})
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	if meta.CaseCount != 1 {
		t.Fatalf("expected case_count=1, got %d", meta.CaseCount)
	}
	if meta.DocumentCount != 1 {
		t.Fatalf("expected document_count=1, got %d", meta.DocumentCount)
	}

	hasMeta, hasDump, hasVault := archiveHasEntries(t, archivePath)
	if !hasMeta || !hasDump || !hasVault {
		t.Fatalf("backup archive missing required entries (meta=%v dump=%v vault=%v)", hasMeta, hasDump, hasVault)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM vault_documents`); err != nil {
		t.Fatalf("delete vault metadata: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM cases`); err != nil {
		t.Fatalf("delete cases: %v", err)
	}
	if err := os.Remove(fullPath); err != nil {
		t.Fatalf("remove vault file: %v", err)
	}

	result, err := svc.Restore(ctx, backup.RestoreOptions{InputPath: archivePath, Confirm: true})
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if result.CasesCount != 1 {
		t.Fatalf("expected restored cases=1, got %d", result.CasesCount)
	}
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("expected restored vault file to exist: %v", err)
	}

	if _, err := os.Stat(archivePath + ".pre-restore.tar.gz"); err != nil {
		t.Fatalf("expected pre-restore backup to exist: %v", err)
	}
}

func TestRestoreWithoutConfirmPrintsPlan(t *testing.T) {
	requirePGTools(t)

	db, dsn, cleanup := setupPostgresWithMigrationsDSN(t)
	defer cleanup()
	ctx := context.Background()

	vaultRoot := filepath.Join(t.TempDir(), "vault")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	svc := backup.NewService(db, dsn, vaultRoot)
	if _, err := svc.Backup(ctx, backup.BackupOptions{OutputPath: archivePath}); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	_, err := svc.Restore(ctx, backup.RestoreOptions{InputPath: archivePath, Confirm: false})
	if err == nil {
		t.Fatalf("expected restore to require --confirm")
	}
	if err != nil && !isConfirmErr(err) {
		t.Fatalf("expected confirm error, got %v", err)
	}
}

func TestVerifyDetectsCorruptArchive(t *testing.T) {
	svc := backup.NewService(nil, "", t.TempDir())
	bad := filepath.Join(t.TempDir(), "bad.tar.gz")
	if err := os.WriteFile(bad, []byte("not a gzip"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	_, err := svc.Verify(context.Background(), bad)
	if err == nil {
		t.Fatalf("expected verify to fail for corrupt archive")
	}
}

func TestBackupVerifyReportsMetadata(t *testing.T) {
	requirePGTools(t)

	db, dsn, cleanup := setupPostgresWithMigrationsDSN(t)
	defer cleanup()
	ctx := context.Background()

	tenantID, principalID, caseTypeID, workflowID := insertBaseFixtures(t, ctx, db, "verify")
	_ = insertCase(t, ctx, db, tenantID, principalID, caseTypeID, workflowID, "VRF-001")

	vaultRoot := filepath.Join(t.TempDir(), "vault")
	archivePath := filepath.Join(t.TempDir(), "verify.tar.gz")
	svc := backup.NewService(db, dsn, vaultRoot)
	if _, err := svc.Backup(ctx, backup.BackupOptions{OutputPath: archivePath}); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	res, err := svc.Verify(ctx, archivePath)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if res.Status != "OK" {
		t.Fatalf("expected status OK, got %s", res.Status)
	}
	if res.Metadata.CaseCount != 1 {
		t.Fatalf("expected case_count=1, got %d", res.Metadata.CaseCount)
	}
}

func requirePGTools(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not found on PATH")
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skip("pg_restore not found on PATH")
	}
}

func setupPostgresWithMigrationsDSN(t *testing.T) (*sql.DB, string, func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "pgvector/pgvector:pg17",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "aceryx",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("resolve container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("resolve container port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://postgres:postgres@%s:%s/aceryx?sslmode=disable", host, port.Port())
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("open db: %v", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		t.Fatalf("ping db: %v", err)
	}

	runner := internalmigrations.NewRunner(db)
	if err := runner.Apply(ctx); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		t.Fatalf("apply migrations: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = container.Terminate(context.Background())
	}
	return db, dsn, cleanup
}

func archiveHasEntries(t *testing.T, archivePath string) (bool, bool, bool) {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("open gzip: %v", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	hasMeta, hasDump, hasVault := false, false, false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		switch hdr.Name {
		case "backup_meta.json":
			hasMeta = true
			var meta backup.Metadata
			if err := json.NewDecoder(tr).Decode(&meta); err != nil {
				t.Fatalf("decode metadata: %v", err)
			}
		case "postgres.dump":
			hasDump = true
		default:
			if hdr.Name == "vault/" || len(hdr.Name) >= 6 && hdr.Name[:6] == "vault/" {
				hasVault = true
			}
		}
	}
	return hasMeta, hasDump, hasVault
}

func isConfirmErr(err error) bool {
	return err != nil && (errors.Is(err, backup.ErrRestoreNeedsConfirm) ||
		strings.Contains(err.Error(), backup.ErrRestoreNeedsConfirm.Error()))
}
