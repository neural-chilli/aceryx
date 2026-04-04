package backup

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

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
