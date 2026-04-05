package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

type Driver struct{}

func New() *Driver { return &Driver{} }

func (d *Driver) ID() string          { return "postgres" }
func (d *Driver) DisplayName() string { return "PostgreSQL" }

func (d *Driver) Connect(ctx context.Context, config drivers.DBConfig) (*sql.DB, error) {
	_ = ctx
	host := config.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := config.Port
	if port == 0 {
		port = 5432
	}
	ssl := config.SSLMode
	if ssl == "" {
		ssl = "prefer"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   config.Database,
	}
	q := url.Values{}
	q.Set("sslmode", ssl)
	u.RawQuery = q.Encode()

	db, err := sql.Open("pgx", u.String())
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	applyPoolConfig(db, config)
	return db, nil
}

func (d *Driver) Ping(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (d *Driver) Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}

func applyPoolConfig(db *sql.DB, cfg drivers.DBConfig) {
	max := cfg.MaxConns
	if max <= 0 {
		max = 10
	}
	idle := cfg.IdleConns
	if idle < 0 {
		idle = 0
	}
	if idle == 0 {
		idle = 2
	}
	db.SetMaxOpenConns(max)
	db.SetMaxIdleConns(idle)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
}

func DSN(config drivers.DBConfig) string {
	host := config.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := config.Port
	if port == 0 {
		port = 5432
	}
	ssl := config.SSLMode
	if ssl == "" {
		ssl = "prefer"
	}
	return "host=" + host + " port=" + strconv.Itoa(port) + " dbname=" + config.Database + " user=" + config.User + " password=" + config.Password + " sslmode=" + ssl
}
