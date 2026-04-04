package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	internalmigrations "github.com/neural-chilli/aceryx/internal/migrations"
)

var ErrRestoreNeedsConfirm = errors.New("restore requires --confirm")

type Service struct {
	db         *sql.DB
	dbURL      string
	vaultPath  string
	pgDumpBin  string
	pgRestore  string
	now        func() time.Time
	runCommand func(ctx context.Context, name string, args []string, env []string) error
}

func NewService(db *sql.DB, dbURL, vaultPath string) *Service {
	return &Service{
		db:        db,
		dbURL:     dbURL,
		vaultPath: vaultPath,
		pgDumpBin: "pg_dump",
		pgRestore: "pg_restore",
		now: func() time.Time {
			return time.Now().UTC()
		},
		runCommand: runCommand,
	}
}

func (s *Service) Backup(ctx context.Context, opts BackupOptions) (Metadata, error) {
	if strings.TrimSpace(opts.OutputPath) == "" {
		return Metadata{}, fmt.Errorf("output path is required")
	}

	if opts.Pause {
		if err := EnableMaintenanceMode(s.vaultPath); err != nil {
			return Metadata{}, fmt.Errorf("enable maintenance mode: %w", err)
		}
		defer func() {
			_ = DisableMaintenanceMode(s.vaultPath)
		}()
	}

	meta, err := s.collectMetadata(ctx, opts.TenantID)
	if err != nil {
		return Metadata{}, err
	}

	tmpDir, err := os.MkdirTemp("", "aceryx-backup-*")
	if err != nil {
		return Metadata{}, fmt.Errorf("create backup temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dumpPath := filepath.Join(tmpDir, "postgres.dump")
	if opts.TenantID == nil {
		if err := s.pgDump(ctx, s.dbURL, dumpPath); err != nil {
			return Metadata{}, err
		}
	} else {
		if err := s.pgDumpTenant(ctx, s.dbURL, dumpPath, *opts.TenantID); err != nil {
			return Metadata{}, err
		}
	}

	vaultRoot := filepath.Join(tmpDir, "vault")
	if err := s.snapshotVault(vaultRoot, opts.TenantID); err != nil {
		return Metadata{}, err
	}

	metaPath := filepath.Join(tmpDir, "backup_meta.json")
	meta.SizeBytes = 0
	if err := writeMetadata(metaPath, meta); err != nil {
		return Metadata{}, err
	}

	if err := buildArchive(opts.OutputPath, tmpDir); err != nil {
		return Metadata{}, err
	}

	st, err := os.Stat(opts.OutputPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("stat backup archive: %w", err)
	}
	meta.SizeBytes = st.Size()

	if err := writeMetadata(metaPath, meta); err != nil {
		return Metadata{}, err
	}
	if err := buildArchive(opts.OutputPath, tmpDir); err != nil {
		return Metadata{}, err
	}

	finalInfo, err := os.Stat(opts.OutputPath)
	if err == nil {
		meta.SizeBytes = finalInfo.Size()
	}

	return meta, nil
}

func (s *Service) Restore(ctx context.Context, opts RestoreOptions) (RestoreResult, error) {
	if strings.TrimSpace(opts.InputPath) == "" {
		return RestoreResult{}, fmt.Errorf("input path is required")
	}

	targetDB := s.dbURL
	if strings.TrimSpace(opts.TargetDBURL) != "" {
		targetDB = opts.TargetDBURL
	}
	if targetDB == "" {
		return RestoreResult{}, fmt.Errorf("target database url is required")
	}

	extracted, err := extractArchive(opts.InputPath)
	if err != nil {
		return RestoreResult{}, err
	}
	defer func() { _ = os.RemoveAll(extracted.dir) }()

	if extracted.meta.Version != BackupFormatVersion {
		return RestoreResult{}, fmt.Errorf("unsupported backup format version %q", extracted.meta.Version)
	}

	if !opts.Confirm {
		return RestoreResult{}, fmt.Errorf("%w: would restore backup %s to target database and replace vault path %s", ErrRestoreNeedsConfirm, opts.InputPath, s.vaultPath)
	}

	preRestoreBackup := opts.InputPath + ".pre-restore.tar.gz"
	preSvc := NewService(s.db, targetDB, s.vaultPath)
	if _, err := preSvc.Backup(ctx, BackupOptions{OutputPath: preRestoreBackup}); err != nil {
		return RestoreResult{}, fmt.Errorf("create pre-restore backup: %w", err)
	}

	targetDBConn, err := sql.Open("pgx", targetDB)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("open target db: %w", err)
	}
	defer func() { _ = targetDBConn.Close() }()

	if err := targetDBConn.PingContext(ctx); err != nil {
		return RestoreResult{}, fmt.Errorf("ping target db: %w", err)
	}

	if err := resetSchema(ctx, targetDBConn); err != nil {
		return RestoreResult{}, err
	}
	if err := s.pgRestoreDump(ctx, targetDB, extracted.dumpPath, extracted.meta.PostgresVersion); err != nil {
		return RestoreResult{}, err
	}
	if err := replaceVaultContents(s.vaultPath, extracted.vaultPath); err != nil {
		return RestoreResult{}, err
	}

	latest, err := internalmigrations.LatestVersion()
	if err != nil {
		return RestoreResult{}, fmt.Errorf("read latest migration version: %w", err)
	}

	migrated := false
	if extracted.meta.SchemaVersion < latest {
		runner := internalmigrations.NewRunner(targetDBConn)
		if err := runner.Apply(ctx); err != nil {
			return RestoreResult{}, fmt.Errorf("apply migrations after restore: %w", err)
		}
		migrated = true
	}

	casesCount, docsCount, verified, sampled, err := verifyRestored(ctx, targetDBConn, s.vaultPath)
	if err != nil {
		return RestoreResult{}, err
	}

	result := RestoreResult{
		CasesCount:         casesCount,
		DocumentsCount:     docsCount,
		SchemaVersion:      latest,
		MigratedFrom:       extracted.meta.SchemaVersion,
		MigrationsApplied:  migrated,
		VaultFilesVerified: verified,
		VaultFilesSampled:  sampled,
	}
	return result, nil
}

func (s *Service) Verify(ctx context.Context, inputPath string) (VerifyResult, error) {
	extracted, err := extractArchive(inputPath)
	if err != nil {
		return VerifyResult{Status: "CORRUPT"}, err
	}
	defer func() { _ = os.RemoveAll(extracted.dir) }()

	if extracted.meta.Version != BackupFormatVersion {
		return VerifyResult{Status: "CORRUPT"}, fmt.Errorf("unsupported backup format version %q", extracted.meta.Version)
	}

	if err := s.pgRestoreList(ctx, extracted.dumpPath, extracted.meta.PostgresVersion); err != nil {
		return VerifyResult{Status: "CORRUPT"}, err
	}

	files, bytes, err := countVaultFiles(extracted.vaultPath)
	if err != nil {
		return VerifyResult{Status: "CORRUPT"}, err
	}

	return VerifyResult{
		Metadata:       extracted.meta,
		DumpValid:      true,
		VaultFileCount: files,
		VaultSizeBytes: bytes,
		Status:         "OK",
	}, nil
}
