package mcp

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Refresher struct {
	cache    *ToolCache
	manager  *Manager
	interval time.Duration
	staleMax time.Duration

	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
}

func NewRefresher(cache *ToolCache, manager *Manager, interval, staleMax time.Duration) *Refresher {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if staleMax <= 0 {
		staleMax = 7 * 24 * time.Hour
	}
	return &Refresher{cache: cache, manager: manager, interval: interval, staleMax: staleMax}
}

func (r *Refresher) Start(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	rctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.running = true
	r.mu.Unlock()

	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			r.refreshOnce(rctx)
			select {
			case <-rctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (r *Refresher) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	r.cancel = nil
	r.running = false
}

func (r *Refresher) refreshOnce(ctx context.Context) {
	if r == nil || r.cache == nil || r.manager == nil {
		return
	}
	servers, err := r.cache.ListAllServers(ctx)
	if err != nil {
		slog.Warn("mcp cache refresh failed to list servers", "error", err)
		return
	}
	for _, server := range servers {
		if strings.TrimSpace(server.ServerURL) == "" || server.TenantID == uuid.Nil {
			continue
		}
		tools, err := r.manager.DiscoverTools(ctx, server.TenantID, server.ServerURL, AuthConfig{Type: "none"})
		if err != nil {
			_ = r.cache.MarkStale(ctx, server.TenantID, server.ServerURL)
			slog.Warn("mcp cache refresh server failed", "tenant_id", server.TenantID.String(), "server_url", server.ServerURL, "error", err)
			if time.Since(server.LastDiscovered) > r.staleMax {
				slog.Warn("mcp cache stale for too long", "tenant_id", server.TenantID.String(), "server_url", server.ServerURL, "stale_hours", int(time.Since(server.LastDiscovered).Hours()))
			}
			continue
		}
		if err := r.cache.SetTools(ctx, server.TenantID, server.ServerURL, tools); err != nil {
			slog.Warn("mcp cache refresh failed to persist", "tenant_id", server.TenantID.String(), "server_url", server.ServerURL, "error", err)
		}
	}
}
