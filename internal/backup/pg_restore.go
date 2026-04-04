package backup

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

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
