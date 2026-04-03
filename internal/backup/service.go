package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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

type extractedArchive struct {
	dir       string
	meta      Metadata
	dumpPath  string
	vaultPath string
}

func extractArchive(inputPath string) (extractedArchive, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return extractedArchive{}, fmt.Errorf("open backup archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return extractedArchive{}, fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tmpDir, err := os.MkdirTemp("", "aceryx-restore-*")
	if err != nil {
		return extractedArchive{}, fmt.Errorf("create restore temp dir: %w", err)
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			return extractedArchive{}, fmt.Errorf("read archive: %w", err)
		}
		if hdr == nil {
			continue
		}

		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") {
			_ = os.RemoveAll(tmpDir)
			return extractedArchive{}, fmt.Errorf("archive contains invalid path %q", hdr.Name)
		}

		targetPath := filepath.Join(tmpDir, clean)
		if !strings.HasPrefix(targetPath, tmpDir) {
			_ = os.RemoveAll(tmpDir)
			return extractedArchive{}, fmt.Errorf("archive path escapes extraction root %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("create extracted directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("create extracted file parent: %w", err)
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("create extracted file: %w", err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("write extracted file: %w", err)
			}
			if err := out.Close(); err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("close extracted file: %w", err)
			}
		}
	}

	metaPath := filepath.Join(tmpDir, "backup_meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return extractedArchive{}, fmt.Errorf("read backup_meta.json: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		_ = os.RemoveAll(tmpDir)
		return extractedArchive{}, fmt.Errorf("decode backup_meta.json: %w", err)
	}

	dumpPath := filepath.Join(tmpDir, "postgres.dump")
	if _, err := os.Stat(dumpPath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return extractedArchive{}, fmt.Errorf("backup missing postgres.dump: %w", err)
	}

	vaultPath := filepath.Join(tmpDir, "vault")
	if _, err := os.Stat(vaultPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(vaultPath, 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				return extractedArchive{}, fmt.Errorf("create empty extracted vault dir: %w", err)
			}
		} else {
			_ = os.RemoveAll(tmpDir)
			return extractedArchive{}, fmt.Errorf("stat extracted vault path: %w", err)
		}
	}

	return extractedArchive{dir: tmpDir, meta: meta, dumpPath: dumpPath, vaultPath: vaultPath}, nil
}

func buildArchive(outputPath, sourceDir string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output parent directory: %w", err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create backup archive: %w", err)
	}
	defer func() { _ = out.Close() }()

	gz := gzip.NewWriter(out)
	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	entries := []string{"backup_meta.json", "postgres.dump", "vault"}
	for _, name := range entries {
		full := filepath.Join(sourceDir, name)
		if _, err := os.Stat(full); err != nil {
			return fmt.Errorf("required backup item %q missing: %w", name, err)
		}
		if err := addPathToTar(tw, full, name); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close backup archive: %w", err)
	}

	return nil
}

func addPathToTar(tw *tar.Writer, fullPath, archivePath string) error {
	info, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("stat backup item %q: %w", fullPath, err)
	}

	if info.IsDir() {
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("create tar header for directory %q: %w", fullPath, err)
		}
		hdr.Name = filepath.ToSlash(archivePath)
		if !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write tar directory header %q: %w", fullPath, err)
		}

		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return fmt.Errorf("read backup directory %q: %w", fullPath, err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			childFull := filepath.Join(fullPath, entry.Name())
			childArchive := filepath.Join(archivePath, entry.Name())
			if err := addPathToTar(tw, childFull, childArchive); err != nil {
				return err
			}
		}
		return nil
	}

	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("create tar header for file %q: %w", fullPath, err)
	}
	hdr.Name = filepath.ToSlash(archivePath)
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar file header %q: %w", fullPath, err)
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return fmt.Errorf("open backup file %q: %w", fullPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("write backup file %q to archive: %w", fullPath, err)
	}

	return nil
}

func writeMetadata(path string, meta Metadata) error {
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal backup metadata: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write backup metadata file: %w", err)
	}
	return nil
}

func (s *Service) snapshotVault(destRoot string, tenantID *uuid.UUID) error {
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return fmt.Errorf("create temporary vault snapshot directory: %w", err)
	}

	if s.vaultPath == "" {
		return nil
	}

	source := s.vaultPath
	if tenantID != nil {
		source = filepath.Join(source, tenantID.String())
	}
	st, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat vault source %q: %w", source, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("vault source %q is not a directory", source)
	}

	if tenantID != nil {
		return copyTree(source, filepath.Join(destRoot, tenantID.String()))
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read vault root directory: %w", err)
	}
	for _, entry := range entries {
		if err := copyTree(filepath.Join(source, entry.Name()), filepath.Join(destRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source path %q: %w", src, err)
	}
	if st.IsDir() {
		if err := os.MkdirAll(dst, st.Mode().Perm()); err != nil {
			return fmt.Errorf("create destination directory %q: %w", dst, err)
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return fmt.Errorf("read source directory %q: %w", src, err)
		}
		for _, entry := range entries {
			if err := copyTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination file parent %q: %w", dst, err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, st.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy source file %q: %w", src, err)
	}
	return nil
}

func (s *Service) collectMetadata(ctx context.Context, tenantID *uuid.UUID) (Metadata, error) {
	meta := Metadata{
		Version:       BackupFormatVersion,
		AceryxVersion: AceryxVersion,
		CreatedAt:     s.now(),
		TenantFilter:  tenantID,
	}

	if err := s.db.QueryRowContext(ctx, `SELECT current_setting('server_version')`).Scan(&meta.PostgresVersion); err != nil {
		return Metadata{}, fmt.Errorf("query postgres version: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&meta.SchemaVersion); err != nil {
		return Metadata{}, fmt.Errorf("query schema version: %w", err)
	}

	if tenantID == nil {
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases`).Scan(&meta.CaseCount); err != nil {
			return Metadata{}, fmt.Errorf("query case count: %w", err)
		}
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE deleted_at IS NULL`).Scan(&meta.DocumentCount); err != nil {
			return Metadata{}, fmt.Errorf("query document count: %w", err)
		}
		return meta, nil
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases WHERE tenant_id = $1`, *tenantID).Scan(&meta.CaseCount); err != nil {
		return Metadata{}, fmt.Errorf("query tenant case count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE tenant_id = $1 AND deleted_at IS NULL`, *tenantID).Scan(&meta.DocumentCount); err != nil {
		return Metadata{}, fmt.Errorf("query tenant document count: %w", err)
	}
	return meta, nil
}

func (s *Service) pgDump(ctx context.Context, dbURL, outPath string) error {
	details, err := parseConn(dbURL)
	if err != nil {
		return err
	}

	args := []string{
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--host", details.Host,
		"--port", details.Port,
		"--username", details.User,
		"--dbname", details.DBName,
		"--file", outPath,
	}
	if err := s.runCommand(ctx, s.pgDumpBin, args, details.Env()); err != nil {
		if isPGVersionMismatchError(err) {
			major, parseErr := extractServerMajorFromError(err)
			if parseErr != nil {
				return fmt.Errorf("run pg_dump: %w", err)
			}
			if dockerErr := s.pgDumpViaDocker(ctx, details, outPath, major); dockerErr == nil {
				return nil
			}
		}
		return fmt.Errorf("run pg_dump: %w", err)
	}
	return nil
}

var serverVersionRegexp = regexp.MustCompile(`server version:\s*([0-9]+)\.`)

func isPGVersionMismatchError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "server version mismatch")
}

func extractServerMajorFromError(err error) (int, error) {
	if err == nil {
		return 0, fmt.Errorf("nil error")
	}
	matches := serverVersionRegexp.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return 0, fmt.Errorf("server major version not found in error")
	}
	major, convErr := strconv.Atoi(matches[1])
	if convErr != nil {
		return 0, fmt.Errorf("parse server major version: %w", convErr)
	}
	return major, nil
}

func (s *Service) pgDumpViaDocker(ctx context.Context, details connDetails, outPath string, serverMajor int) error {
	if serverMajor <= 0 {
		return fmt.Errorf("invalid server major version %d", serverMajor)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not available for pg_dump fallback: %w", err)
	}

	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return fmt.Errorf("resolve absolute dump path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absOut), 0o755); err != nil {
		return fmt.Errorf("create dump directory: %w", err)
	}

	outDir := filepath.Dir(absOut)
	outFile := filepath.Base(absOut)

	args := []string{
		"run", "--rm",
		"--network", "host",
		"-v", fmt.Sprintf("%s:/backup", outDir),
	}
	if details.Password != "" {
		args = append(args, "-e", "PGPASSWORD="+details.Password)
	}

	host := details.Host
	if host == "" {
		host = "127.0.0.1"
	}

	args = append(args,
		fmt.Sprintf("postgres:%d", serverMajor),
		"pg_dump",
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--host", host,
		"--port", details.Port,
		"--username", details.User,
		"--dbname", details.DBName,
		"--file", "/backup/"+outFile,
	)

	if err := s.runCommand(ctx, "docker", args, nil); err != nil {
		return fmt.Errorf("run dockerized pg_dump: %w", err)
	}
	return nil
}

func (s *Service) pgDumpTenant(ctx context.Context, dbURL, outPath string, tenantID uuid.UUID) error {
	tmpDir, err := os.MkdirTemp("", "aceryx-tenant-dump-*")
	if err != nil {
		return fmt.Errorf("create tenant dump temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	fullDump := filepath.Join(tmpDir, "full.dump")
	if err := s.pgDump(ctx, dbURL, fullDump); err != nil {
		return err
	}

	details, err := parseConn(dbURL)
	if err != nil {
		return err
	}
	tmpDBName := fmt.Sprintf("aceryx_tenant_backup_%d", time.Now().UnixNano())
	if err := createDatabase(ctx, details, tmpDBName); err != nil {
		return err
	}
	defer func() {
		_ = dropDatabase(context.Background(), details, tmpDBName)
	}()

	tmpDBURL := details.WithDatabase(tmpDBName)
	if err := s.pgRestoreDump(ctx, tmpDBURL, fullDump, ""); err != nil {
		return err
	}

	tmpDB, err := sql.Open("pgx", tmpDBURL)
	if err != nil {
		return fmt.Errorf("open temporary restore database: %w", err)
	}
	defer func() { _ = tmpDB.Close() }()

	if err := tmpDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping temporary restore database: %w", err)
	}
	if err := pruneOtherTenants(ctx, tmpDB, tenantID); err != nil {
		return err
	}

	if err := s.pgDump(ctx, tmpDBURL, outPath); err != nil {
		return err
	}
	return nil
}

func pruneOtherTenants(ctx context.Context, db *sql.DB, tenantID uuid.UUID) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM tenants WHERE id <> $1`, tenantID); err != nil {
		return fmt.Errorf("prune other tenants from tenants table: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
SELECT table_schema, table_name
FROM information_schema.columns
WHERE table_schema = 'public' AND column_name = 'tenant_id'
GROUP BY table_schema, table_name
ORDER BY table_name
`)
	if err != nil {
		return fmt.Errorf("query tenant scoped tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tables := make([][2]string, 0)
	for rows.Next() {
		var schema, table string
		if err := rows.Scan(&schema, &table); err != nil {
			return fmt.Errorf("scan tenant scoped table row: %w", err)
		}
		tables = append(tables, [2]string{schema, table})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tenant scoped tables: %w", err)
	}

	for _, entry := range tables {
		schema, table := entry[0], entry[1]
		stmt := fmt.Sprintf(`DELETE FROM %s.%s WHERE tenant_id IS DISTINCT FROM $1`, quoteIdentifier(schema), quoteIdentifier(table))
		if _, err := db.ExecContext(ctx, stmt, tenantID); err != nil {
			return fmt.Errorf("prune other tenant rows from %s.%s: %w", schema, table, err)
		}
	}

	return nil
}

func (s *Service) pgRestoreDump(ctx context.Context, dbURL, dumpPath, dumpPostgresVersion string) error {
	details, err := parseConn(dbURL)
	if err != nil {
		return err
	}
	args := []string{
		"--clean",
		"--if-exists",
		"--no-owner",
		"--no-privileges",
		"--host", details.Host,
		"--port", details.Port,
		"--username", details.User,
		"--dbname", details.DBName,
		dumpPath,
	}
	if err := s.runCommand(ctx, s.pgRestore, args, details.Env()); err != nil {
		if isPGDumpHeaderUnsupportedError(err) {
			major, parseErr := parseMajorFromVersionString(dumpPostgresVersion)
			if parseErr == nil {
				if dockerErr := s.pgRestoreViaDocker(ctx, details, dumpPath, major, false); dockerErr == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("run pg_restore: %w", err)
	}
	return nil
}

func (s *Service) pgRestoreList(ctx context.Context, dumpPath, dumpPostgresVersion string) error {
	if err := s.runCommand(ctx, s.pgRestore, []string{"--list", dumpPath}, nil); err != nil {
		if isPGDumpHeaderUnsupportedError(err) {
			major, parseErr := parseMajorFromVersionString(dumpPostgresVersion)
			if parseErr == nil {
				noopDetails := connDetails{}
				if dockerErr := s.pgRestoreViaDocker(ctx, noopDetails, dumpPath, major, true); dockerErr == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("validate postgres dump with pg_restore --list: %w", err)
	}
	return nil
}

func isPGDumpHeaderUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unsupported version (") && strings.Contains(msg, "file header")
}

func parseMajorFromVersionString(v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, fmt.Errorf("empty postgres version")
	}
	re := regexp.MustCompile(`^([0-9]+)`)
	m := re.FindStringSubmatch(v)
	if len(m) < 2 {
		return 0, fmt.Errorf("parse major from version %q", v)
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parse major from version %q: %w", v, err)
	}
	return major, nil
}

func (s *Service) pgRestoreViaDocker(ctx context.Context, details connDetails, dumpPath string, dumpMajor int, listOnly bool) error {
	if dumpMajor <= 0 {
		return fmt.Errorf("invalid dump major version %d", dumpMajor)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not available for pg_restore fallback: %w", err)
	}

	absDump, err := filepath.Abs(dumpPath)
	if err != nil {
		return fmt.Errorf("resolve absolute dump path: %w", err)
	}
	dumpDir := filepath.Dir(absDump)
	dumpFile := filepath.Base(absDump)

	args := []string{
		"run", "--rm",
		"--network", "host",
		"-v", fmt.Sprintf("%s:/backup", dumpDir),
	}
	if details.Password != "" {
		args = append(args, "-e", "PGPASSWORD="+details.Password)
	}
	args = append(args, fmt.Sprintf("postgres:%d", dumpMajor), "pg_restore")

	if listOnly {
		args = append(args, "--list", "/backup/"+dumpFile)
		return s.runCommand(ctx, "docker", args, nil)
	}

	host := details.Host
	if host == "" {
		host = "127.0.0.1"
	}
	args = append(args,
		"--clean",
		"--if-exists",
		"--no-owner",
		"--no-privileges",
		"--host", host,
		"--port", details.Port,
		"--username", details.User,
		"--dbname", details.DBName,
		"/backup/"+dumpFile,
	)
	return s.runCommand(ctx, "docker", args, nil)
}

func runCommand(ctx context.Context, name string, args []string, env []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

type connDetails struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (c connDetails) Env() []string {
	if c.Password == "" {
		return nil
	}
	return []string{"PGPASSWORD=" + c.Password}
}

func (c connDetails) WithDatabase(name string) string {
	u := &url.URL{
		Scheme: "postgres",
		Host:   c.Host + ":" + c.Port,
		Path:   "/" + name,
	}
	if c.User != "" {
		if c.Password != "" {
			u.User = url.UserPassword(c.User, c.Password)
		} else {
			u.User = url.User(c.User)
		}
	}
	q := u.Query()
	if c.SSLMode != "" {
		q.Set("sslmode", c.SSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func parseConn(dbURL string) (connDetails, error) {
	u, err := url.Parse(dbURL)
	if err != nil {
		return connDetails{}, fmt.Errorf("parse database url: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return connDetails{}, fmt.Errorf("unsupported database url scheme %q", u.Scheme)
	}

	user := ""
	password := ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	if user == "" {
		user = os.Getenv("PGUSER")
	}
	if user == "" {
		user = "postgres"
	}

	port := u.Port()
	if port == "" {
		port = "5432"
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return connDetails{}, fmt.Errorf("database name is required")
	}

	sslmode := u.Query().Get("sslmode")
	if sslmode == "" {
		sslmode = "disable"
	}

	return connDetails{
		Host:     u.Hostname(),
		Port:     port,
		User:     user,
		Password: password,
		DBName:   dbName,
		SSLMode:  sslmode,
	}, nil
}

func createDatabase(ctx context.Context, details connDetails, dbName string) error {
	adminDBURL := details.WithDatabase("postgres")
	db, err := sql.Open("pgx", adminDBURL)
	if err != nil {
		return fmt.Errorf("open admin database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %s`, quoteIdentifier(dbName))); err != nil {
		return fmt.Errorf("create temporary database %q: %w", dbName, err)
	}
	return nil
}

func dropDatabase(ctx context.Context, details connDetails, dbName string) error {
	adminDBURL := details.WithDatabase("postgres")
	db, err := sql.Open("pgx", adminDBURL)
	if err != nil {
		return fmt.Errorf("open admin database for drop: %w", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, dbName); err != nil {
		return fmt.Errorf("terminate temporary database connections: %w", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdentifier(dbName))); err != nil {
		return fmt.Errorf("drop temporary database %q: %w", dbName, err)
	}
	return nil
}

func quoteIdentifier(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
}

func resetSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE`); err != nil {
		return fmt.Errorf("drop public schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA public`); err != nil {
		return fmt.Errorf("create public schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto`); err != nil {
		return fmt.Errorf("enable pgcrypto extension: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return fmt.Errorf("enable vector extension: %w", err)
	}
	return nil
}

func replaceVaultContents(destRoot, srcRoot string) error {
	if strings.TrimSpace(destRoot) == "" {
		return fmt.Errorf("vault path is required for restore")
	}
	if err := os.RemoveAll(destRoot); err != nil {
		return fmt.Errorf("remove existing vault contents: %w", err)
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return fmt.Errorf("create vault destination root: %w", err)
	}
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return fmt.Errorf("read extracted vault directory: %w", err)
	}
	for _, entry := range entries {
		if err := copyTree(filepath.Join(srcRoot, entry.Name()), filepath.Join(destRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func verifyRestored(ctx context.Context, db *sql.DB, vaultPath string) (int64, int64, int, int, error) {
	var casesCount int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases`).Scan(&casesCount); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("count restored cases: %w", err)
	}
	var docCount int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE deleted_at IS NULL`).Scan(&docCount); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("count restored documents: %w", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT storage_uri FROM vault_documents WHERE deleted_at IS NULL`)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("load restored vault uris: %w", err)
	}
	defer func() { _ = rows.Close() }()

	uris := make([]string, 0)
	for rows.Next() {
		var uri string
		if err := rows.Scan(&uri); err != nil {
			return 0, 0, 0, 0, fmt.Errorf("scan restored vault uri: %w", err)
		}
		uris = append(uris, uri)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("iterate restored vault uris: %w", err)
	}

	sampled := len(uris)
	if sampled > 25 {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		r.Shuffle(len(uris), func(i, j int) { uris[i], uris[j] = uris[j], uris[i] })
		sampled = 25
		uris = uris[:sampled]
	}

	verified := 0
	for _, uri := range uris {
		if _, err := os.Stat(filepath.Join(vaultPath, filepath.FromSlash(uri))); err == nil {
			verified++
		}
	}

	return casesCount, docCount, verified, sampled, nil
}

func countVaultFiles(vaultDir string) (int, int64, error) {
	count := 0
	var bytes int64
	err := filepath.WalkDir(vaultDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		count++
		bytes += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("walk vault files: %w", err)
	}
	return count, bytes, nil
}
