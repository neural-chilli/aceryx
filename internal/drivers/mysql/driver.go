package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

type Driver struct{}

func New() *Driver { return &Driver{} }

func (d *Driver) ID() string          { return "mysql" }
func (d *Driver) DisplayName() string { return "MySQL" }

func (d *Driver) Connect(ctx context.Context, config drivers.DBConfig) (*sql.DB, error) {
	_ = ctx
	host := config.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := config.Port
	if port == 0 {
		port = 3306
	}
	cfg := mysql.Config{
		User:                 config.User,
		Passwd:               config.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", host, port),
		DBName:               config.Database,
		ParseTime:            true,
		AllowNativePasswords: true,
	}
	if config.SSLMode != "" && config.SSLMode != "disable" {
		cfg.TLSConfig = "true"
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	applyPoolConfig(db, config)
	return db, nil
}

func (d *Driver) Ping(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
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
