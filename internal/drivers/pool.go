package drivers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type PoolManager struct {
	mu    sync.RWMutex
	pools map[poolKey]*managedPool
}

type poolKey struct {
	TenantID   string
	DriverID   string
	ConfigHash string
}

type managedPool struct {
	db        *sql.DB
	config    DBConfig
	createdAt time.Time
	lastUsed  time.Time
}

type PoolStats struct {
	TenantID     string
	DriverID     string
	MaxConns     int
	IdleConns    int
	ActiveConns  int
	WaitCount    int64
	WaitDuration time.Duration
}

func NewPoolManager() *PoolManager {
	return &PoolManager{pools: map[poolKey]*managedPool{}}
}

func (pm *PoolManager) GetOrCreate(ctx context.Context, tenantID string, driver DBDriver, config DBConfig) (*sql.DB, error) {
	if driver == nil {
		return nil, fmt.Errorf("driver is required")
	}
	cfg := withDBDefaults(config)
	hash, err := hashConfig(cfg)
	if err != nil {
		return nil, err
	}
	key := poolKey{TenantID: tenantID, DriverID: driver.ID(), ConfigHash: hash}

	pm.mu.RLock()
	if existing, ok := pm.pools[key]; ok {
		existing.lastUsed = time.Now().UTC()
		db := existing.db
		pm.mu.RUnlock()
		return db, nil
	}
	pm.mu.RUnlock()

	pm.mu.Lock()
	defer pm.mu.Unlock()
	if existing, ok := pm.pools[key]; ok {
		existing.lastUsed = time.Now().UTC()
		return existing.db, nil
	}

	db, err := driver.Connect(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", driver.ID(), err)
	}
	if err := driver.Ping(ctx, db); err != nil {
		_ = driver.Close(db)
		return nil, fmt.Errorf("ping %s: %w", driver.ID(), err)
	}
	pm.pools[key] = &managedPool{
		db:        db,
		config:    cfg,
		createdAt: time.Now().UTC(),
		lastUsed:  time.Now().UTC(),
	}
	return db, nil
}

func (pm *PoolManager) Close(tenantID, driverID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	var errs []error
	for key, pool := range pm.pools {
		if key.TenantID != tenantID || key.DriverID != driverID {
			continue
		}
		if err := pool.db.Close(); err != nil {
			errs = append(errs, err)
		}
		delete(pm.pools, key)
	}
	return errorsJoin(errs)
}

func (pm *PoolManager) CloseAll() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	var errs []error
	for key, pool := range pm.pools {
		if err := pool.db.Close(); err != nil {
			errs = append(errs, err)
		}
		delete(pm.pools, key)
	}
	return errorsJoin(errs)
}

func (pm *PoolManager) Stats() []PoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	stats := make([]PoolStats, 0, len(pm.pools))
	for key, pool := range pm.pools {
		dbstats := pool.db.Stats()
		stats = append(stats, PoolStats{
			TenantID:     key.TenantID,
			DriverID:     key.DriverID,
			MaxConns:     dbstats.MaxOpenConnections,
			IdleConns:    dbstats.Idle,
			ActiveConns:  dbstats.InUse,
			WaitCount:    dbstats.WaitCount,
			WaitDuration: dbstats.WaitDuration,
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].TenantID == stats[j].TenantID {
			return stats[i].DriverID < stats[j].DriverID
		}
		return stats[i].TenantID < stats[j].TenantID
	})
	return stats
}

func withDBDefaults(cfg DBConfig) DBConfig {
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = 10
	}
	if cfg.IdleConns < 0 {
		cfg.IdleConns = 0
	}
	if cfg.IdleConns == 0 {
		cfg.IdleConns = 2
	}
	if cfg.TimeoutSecs <= 0 {
		cfg.TimeoutSecs = 30
	}
	if cfg.RowLimit <= 0 {
		cfg.RowLimit = 10000
	}
	return cfg
}

func hashConfig(cfg DBConfig) (string, error) {
	copyCfg := cfg
	copyCfg.Password = ""
	raw, err := json.Marshal(copyCfg)
	if err != nil {
		return "", fmt.Errorf("marshal db config: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func errorsJoin(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	msg := ""
	for i := range errs {
		if i > 0 {
			msg += "; "
		}
		msg += errs[i].Error()
	}
	return errors.New(msg)
}
