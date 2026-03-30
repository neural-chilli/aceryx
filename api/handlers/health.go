package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/notify"
	"github.com/neural-chilli/aceryx/internal/observability"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HealthHandlers struct {
	db        *sql.DB
	eng       *engine.Engine
	hub       *notify.Hub
	version   string
	startedAt time.Time
	vaultPath string
}

type componentCheck map[string]any

func NewHealthHandlers(db *sql.DB, eng *engine.Engine, hub *notify.Hub) *HealthHandlers {
	vaultPath := os.Getenv("ACERYX_VAULT_ROOT")
	if vaultPath == "" {
		vaultPath = "./data/vault"
	}
	return &HealthHandlers{
		db:        db,
		eng:       eng,
		hub:       hub,
		version:   "1.0.0",
		startedAt: time.Now().UTC(),
		vaultPath: vaultPath,
	}
}

func (h *HealthHandlers) Metrics() http.Handler {
	return promhttp.Handler()
}

func (h *HealthHandlers) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *HealthHandlers) Readiness(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reason": "db_unconfigured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.db.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reason": "postgres_unhealthy"})
		return
	}
	var migrationCount int
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil || migrationCount == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reason": "migrations_not_applied"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (h *HealthHandlers) Health(w http.ResponseWriter, r *http.Request) {
	checks := map[string]componentCheck{}
	healthy := true

	checks["postgres"] = h.checkPostgres(r.Context())
	if checks["postgres"]["status"] != "healthy" {
		healthy = false
	}
	checks["vault"] = h.checkVault()
	if checks["vault"]["status"] != "healthy" {
		healthy = false
	}
	checks["worker_pool"] = h.checkWorkerPool()
	if checks["worker_pool"]["status"] != "healthy" {
		healthy = false
	}
	checks["websocket_hub"] = h.checkWebSocketHub()
	if checks["websocket_hub"]["status"] != "healthy" {
		healthy = false
	}

	status := "healthy"
	code := http.StatusOK
	if !healthy {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]any{
		"status":         status,
		"version":        h.version,
		"uptime_seconds": int(time.Since(h.startedAt).Seconds()),
		"checks":         checks,
	})
}

func (h *HealthHandlers) checkPostgres(ctx context.Context) componentCheck {
	if h.db == nil {
		return componentCheck{"status": "unhealthy", "error": "db_unconfigured"}
	}
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := h.db.PingContext(cctx); err != nil {
		return componentCheck{"status": "unhealthy", "error": err.Error()}
	}
	observability.UpdateDBPoolStats(h.db)
	return componentCheck{"status": "healthy", "latency_ms": time.Since(start).Milliseconds()}
}

func (h *HealthHandlers) checkVault() componentCheck {
	info, err := os.Stat(h.vaultPath)
	if err != nil || !info.IsDir() {
		return componentCheck{"status": "unhealthy", "path": h.vaultPath, "writable": false}
	}
	f, err := os.CreateTemp(h.vaultPath, "health-*.tmp")
	if err != nil {
		return componentCheck{"status": "unhealthy", "path": h.vaultPath, "writable": false}
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return componentCheck{"status": "healthy", "path": h.vaultPath, "writable": true}
}

func (h *HealthHandlers) checkWorkerPool() componentCheck {
	if h.eng == nil {
		return componentCheck{"status": "healthy", "active": 0, "capacity": 0}
	}
	active, capTotal := h.eng.WorkerPoolStats()
	if capTotal > 0 {
		observability.WorkerPoolUtilisation.Set(float64(active) / float64(capTotal))
	}
	return componentCheck{"status": "healthy", "active": active, "capacity": capTotal}
}

func (h *HealthHandlers) checkWebSocketHub() componentCheck {
	if h.hub == nil {
		return componentCheck{"status": "healthy", "connections": 0}
	}
	return componentCheck{"status": "healthy", "connections": h.hub.TotalConnections()}
}

// Health keeps compatibility with earlier tests/consumers.
func Health(w http.ResponseWriter, r *http.Request) {
	NewHealthHandlers(nil, nil, nil).Health(w, r)
}
