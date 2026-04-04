package engine

import (
	"context"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (e *Engine) updateCaseStepStateMetrics(ctx context.Context, tenantID uuid.UUID) {
	if tenantID == uuid.Nil {
		return
	}
	states := []string{"pending", "ready", "active", "completed", "failed", "skipped"}
	for _, state := range states {
		var count int
		if err := e.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE c.tenant_id = $1 AND cs.state = $2
`, tenantID, state).Scan(&count); err == nil {
			observability.CaseStepsTotal.WithLabelValues(tenantID.String(), state).Set(float64(count))
		}
	}
}
