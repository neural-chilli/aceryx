package triggers

import (
	"context"
	"log/slog"
	"time"
)

func enqueueWithBackpressure(ctx context.Context, instance *TriggerInstance, env messageEnvelope, warnAfter time.Duration) {
	if warnAfter <= 0 {
		warnAfter = 60 * time.Second
	}
	select {
	case instance.buffer <- env:
		return
	case <-time.After(warnAfter):
		slog.Warn("trigger buffer full for >60s, pipeline may be overloaded", "trigger_id", instance.id)
		select {
		case instance.buffer <- env:
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}
}
