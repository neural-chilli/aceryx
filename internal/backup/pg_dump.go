package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

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
