package cases

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func (r *ReportsService) StartRefreshTicker(ctx context.Context) {
	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = r.RefreshMaterializedViews(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *ReportsService) RefreshMaterializedViews(ctx context.Context) error {
	for _, view := range []string{"mv_cases_summary", "mv_sla_compliance"} {
		if _, err := r.db.ExecContext(ctx, `REFRESH MATERIALIZED VIEW `+view); err != nil {
			return fmt.Errorf("refresh materialized view %s: %w", view, err)
		}
	}
	return nil
}

func (r *ReportsService) CasesSummary(ctx context.Context, tenantID uuid.UUID, weeks int) ([]CasesSummaryRow, error) {
	if weeks <= 0 {
		weeks = 12
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT period, period + interval '6 day' AS period_end,
       COALESCE(SUM(case_count) FILTER (WHERE status='open'), 0) AS opened,
       COALESCE(SUM(case_count) FILTER (WHERE status='completed'), 0) AS closed,
       COALESCE(SUM(case_count) FILTER (WHERE status='cancelled'), 0) AS cancelled,
       COALESCE(AVG(avg_days), 0)
FROM mv_cases_summary
WHERE tenant_id = $1 AND period >= date_trunc('week', now() - make_interval(weeks => $2::int))
GROUP BY period
ORDER BY period
`, tenantID, weeks)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]CasesSummaryRow, 0)
	for rows.Next() {
		var row CasesSummaryRow
		if err := rows.Scan(&row.PeriodStart, &row.PeriodEnd, &row.Opened, &row.Closed, &row.Cancelled, &row.AvgDaysToClose); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) Ageing(ctx context.Context, tenantID uuid.UUID, thresholds []int) ([]AgeingBracket, error) {
	if len(thresholds) == 0 {
		thresholds = []int{7, 14, 30}
	}
	sort.Ints(thresholds)
	rows, err := r.db.QueryContext(ctx, `
SELECT id, created_at
FROM cases
WHERE tenant_id = $1 AND status IN ('open', 'in_progress')
`, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	b := make([]AgeingBracket, 0, len(thresholds)+1)
	for i := 0; i < len(thresholds); i++ {
		if i == 0 {
			b = append(b, AgeingBracket{Label: "0-" + strconv.Itoa(thresholds[i]) + " days"})
		} else {
			b = append(b, AgeingBracket{Label: strconv.Itoa(thresholds[i-1]) + "-" + strconv.Itoa(thresholds[i]) + " days"})
		}
	}
	b = append(b, AgeingBracket{Label: strconv.Itoa(thresholds[len(thresholds)-1]) + "+ days"})

	now := time.Now()
	for rows.Next() {
		var id uuid.UUID
		var created time.Time
		if err := rows.Scan(&id, &created); err != nil {
			return nil, err
		}
		days := int(now.Sub(created).Hours() / 24)
		idx := len(b) - 1
		for i, th := range thresholds {
			if days <= th {
				idx = i
				break
			}
		}
		b[idx].Count++
		b[idx].CaseIDs = append(b[idx].CaseIDs, id)
	}
	return b, rows.Err()
}

func (r *ReportsService) SLACompliance(ctx context.Context, tenantID uuid.UUID, weeks int) ([]SLAComplianceRow, error) {
	if weeks <= 0 {
		weeks = 12
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT period, total, within_sla
FROM mv_sla_compliance
WHERE tenant_id = $1 AND period >= date_trunc('week', now() - make_interval(weeks => $2::int))
ORDER BY period
`, tenantID, weeks)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]SLAComplianceRow, 0)
	for rows.Next() {
		var row SLAComplianceRow
		if err := rows.Scan(&row.PeriodStart, &row.TotalTasks, &row.CompletedWithinSLA); err != nil {
			return nil, err
		}
		if row.TotalTasks > 0 {
			row.ComplianceRate = float64(row.CompletedWithinSLA) / float64(row.TotalTasks)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) CasesByStage(ctx context.Context, tenantID uuid.UUID, caseType string) ([]StageRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT cs.step_id, COUNT(*)
FROM cases c
JOIN case_steps cs ON cs.case_id = c.id
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1
  AND c.status IN ('open', 'in_progress')
  AND cs.state = 'active'
  AND ($2 = '' OR ct.name = $2)
GROUP BY cs.step_id
ORDER BY cs.step_id
`, tenantID, caseType)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]StageRow, 0)
	for rows.Next() {
		var row StageRow
		if err := rows.Scan(&row.Stage, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) Workload(ctx context.Context, tenantID uuid.UUID) ([]WorkloadRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT p.id, p.name,
       COUNT(*) FILTER (WHERE cs.state = 'active') AS active_tasks,
       COUNT(*) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL AND cs.sla_deadline < now()) AS breached_sla
FROM principals p
LEFT JOIN case_steps cs ON cs.assigned_to = p.id
WHERE p.tenant_id = $1
GROUP BY p.id, p.name
ORDER BY p.name
`, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WorkloadRow, 0)
	for rows.Next() {
		var row WorkloadRow
		if err := rows.Scan(&row.PrincipalID, &row.Name, &row.ActiveTasks, &row.BreachedSLA); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) Decisions(ctx context.Context, tenantID uuid.UUID, weeks int) ([]DecisionRow, error) {
	if weeks <= 0 {
		weeks = 12
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT date_trunc('week', ce.created_at) AS period,
       COUNT(*) FILTER (WHERE actor_type = 'agent' AND ((event_type = 'agent' AND action = 'completed') OR (event_type = 'case' AND action = 'updated'))),
       COUNT(*) FILTER (WHERE actor_type = 'human' AND ((event_type = 'task' AND action = 'completed') OR (event_type = 'case' AND action = 'updated'))),
       COUNT(*) FILTER (WHERE event_type = 'agent' AND action = 'escalated')
FROM case_events ce
JOIN cases c ON c.id = ce.case_id
WHERE c.tenant_id = $1 AND ce.created_at >= date_trunc('week', now() - make_interval(weeks => $2::int))
GROUP BY date_trunc('week', ce.created_at)
ORDER BY period
`, tenantID, weeks)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]DecisionRow, 0)
	for rows.Next() {
		var row DecisionRow
		if err := rows.Scan(&row.PeriodStart, &row.AgentDecisions, &row.HumanDecisions, &row.AgentEscalations); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
