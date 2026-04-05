package httpfw

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimitManager struct {
	mu           sync.RWMutex
	pluginLimits map[limitKey]*rate.Limiter
	domainPauses map[domainKey]time.Time
	tenantLimits map[string]*rate.Limiter
	defaults     RateLimitDefaults
}

type limitKey struct {
	TenantID string
	PluginID string
}

type domainKey struct {
	TenantID string
	Domain   string
}

type RateLimitDefaults struct {
	GlobalPerTenant rate.Limit
	GlobalBurst     int
}

func NewRateLimitManager(defaults RateLimitDefaults) *RateLimitManager {
	if defaults.GlobalPerTenant <= 0 {
		defaults.GlobalPerTenant = rate.Limit(100)
	}
	if defaults.GlobalBurst <= 0 {
		defaults.GlobalBurst = 150
	}
	return &RateLimitManager{
		pluginLimits: make(map[limitKey]*rate.Limiter),
		domainPauses: make(map[domainKey]time.Time),
		tenantLimits: make(map[string]*rate.Limiter),
		defaults:     defaults,
	}
}

// Wait blocks until the request is allowed.
func (rlm *RateLimitManager) Wait(ctx context.Context, tenantID, pluginID, domain string) error {
	if rlm == nil {
		return nil
	}
	if limiter := rlm.getPluginLimiter(tenantID, pluginID); limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return err
		}
	}

	for {
		paused, until := rlm.IsDomainPaused(tenantID, domain)
		if !paused {
			break
		}
		delay := time.Until(until)
		if delay <= 0 {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}

	limiter := rlm.getTenantLimiter(tenantID)
	if limiter == nil {
		return nil
	}
	return limiter.Wait(ctx)
}

// SetPluginLimit configures rate limiting for a specific (tenant, plugin) pair.
func (rlm *RateLimitManager) SetPluginLimit(tenantID, pluginID string, rps float64, burst int) {
	if rlm == nil || rps <= 0 {
		return
	}
	if burst <= 0 {
		burst = 1
	}
	key := limitKey{TenantID: tenantID, PluginID: pluginID}
	rlm.mu.Lock()
	rlm.pluginLimits[key] = rate.NewLimiter(rate.Limit(rps), burst)
	rlm.mu.Unlock()
}

// PauseDomain pauses all requests to a domain for a duration.
func (rlm *RateLimitManager) PauseDomain(tenantID, domain string, until time.Time) {
	if rlm == nil {
		return
	}
	key := domainKey{TenantID: tenantID, Domain: strings.ToLower(strings.TrimSpace(domain))}
	rlm.mu.Lock()
	if current, ok := rlm.domainPauses[key]; !ok || until.After(current) {
		rlm.domainPauses[key] = until
	}
	rlm.mu.Unlock()
}

// IsDomainPaused checks if a domain is currently paused.
func (rlm *RateLimitManager) IsDomainPaused(tenantID, domain string) (bool, time.Time) {
	if rlm == nil {
		return false, time.Time{}
	}
	key := domainKey{TenantID: tenantID, Domain: strings.ToLower(strings.TrimSpace(domain))}
	rlm.mu.RLock()
	until, ok := rlm.domainPauses[key]
	rlm.mu.RUnlock()
	if !ok {
		return false, time.Time{}
	}
	if time.Now().After(until) {
		rlm.mu.Lock()
		if current, exists := rlm.domainPauses[key]; exists && !time.Now().Before(current) {
			delete(rlm.domainPauses, key)
		}
		rlm.mu.Unlock()
		return false, time.Time{}
	}
	return true, until
}

func (rlm *RateLimitManager) getPluginLimiter(tenantID, pluginID string) *rate.Limiter {
	if tenantID == "" || pluginID == "" {
		return nil
	}
	key := limitKey{TenantID: tenantID, PluginID: pluginID}
	rlm.mu.RLock()
	limiter := rlm.pluginLimits[key]
	rlm.mu.RUnlock()
	return limiter
}

func (rlm *RateLimitManager) getTenantLimiter(tenantID string) *rate.Limiter {
	rlm.mu.RLock()
	limiter := rlm.tenantLimits[tenantID]
	rlm.mu.RUnlock()
	if limiter != nil {
		return limiter
	}

	rlm.mu.Lock()
	defer rlm.mu.Unlock()
	if existing := rlm.tenantLimits[tenantID]; existing != nil {
		return existing
	}
	created := rate.NewLimiter(rlm.defaults.GlobalPerTenant, rlm.defaults.GlobalBurst)
	rlm.tenantLimits[tenantID] = created
	return created
}
