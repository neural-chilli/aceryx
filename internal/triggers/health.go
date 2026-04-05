package triggers

import (
	"context"
	"time"
)

type HealthMonitor struct {
	manager       *TriggerManager
	checkInterval time.Duration
}

func NewHealthMonitor(manager *TriggerManager, checkInterval time.Duration) *HealthMonitor {
	if checkInterval <= 0 {
		checkInterval = 5 * time.Second
	}
	return &HealthMonitor{manager: manager, checkInterval: checkInterval}
}

func (hm *HealthMonitor) Start(ctx context.Context) {
	if hm == nil || hm.manager == nil {
		return
	}
	ticker := time.NewTicker(hm.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hm.Check(ctx)
		}
	}
}

func (hm *HealthMonitor) Check(ctx context.Context) {
	if hm == nil || hm.manager == nil {
		return
	}
	items := hm.manager.List()
	for _, item := range items {
		if item.Status == TriggerError {
			go hm.manager.restartWithBackoff(ctx, item.ID)
			continue
		}
		hm.manager.maybeResetBackoff(item.ID)
	}
}
