package cases

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (s *CaseService) SearchCases(ctx context.Context, tenantID uuid.UUID, allowedCaseTypeIDs []uuid.UUID, filter SearchFilter) ([]SearchResult, error) {
	start := time.Now()
	defer func() {
		observability.DBQueryDurationSeconds.WithLabelValues("search").Observe(time.Since(start).Seconds())
	}()
	page, perPage := normalizePage(filter.Page, filter.PerPage)

	query := `
SELECT c.id, c.case_number, ct.name, c.status,
       ts_headline('english', c.data::text, plainto_tsquery('english', $2)) AS headline,
       c.created_at, c.updated_at, c.version
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1
  AND to_tsvector('english', c.data::text) @@ plainto_tsquery('english', $2)
`
	args := []interface{}{tenantID, filter.Query}
	idx := 3

	if len(allowedCaseTypeIDs) > 0 {
		query += fmt.Sprintf(" AND c.case_type_id = ANY($%d)", idx)
		args = append(args, pqUUIDArray(allowedCaseTypeIDs))
		idx++
	}
	if filter.CaseType != "" {
		query += fmt.Sprintf(" AND ct.name = $%d", idx)
		args = append(args, filter.CaseType)
		idx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND c.status = $%d", idx)
		args = append(args, filter.Status)
		idx++
	}

	query += fmt.Sprintf(" ORDER BY c.updated_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search cases query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SearchResult, 0)
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.CaseID, &r.CaseNumber, &r.CaseType, &r.Status, &r.Headline, &r.CreatedAt, &r.UpdatedAt, &r.CaseVersion); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *CaseService) Dashboard(ctx context.Context, tenantID uuid.UUID, filter DashboardFilter) ([]DashboardRow, error) {
	page, perPage := normalizePage(filter.Page, filter.PerPage)

	query := `
SELECT c.id, c.case_number, ct.name, c.status, c.assigned_to, c.priority, c.created_at, c.updated_at,
       COALESCE(
           CASE
             WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) IS NULL THEN 'n/a'
             WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) < now() THEN 'breached'
             WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) < now() + interval '24 hour' THEN 'warning'
             ELSE 'on_track'
           END,
           'n/a'
       ) AS sla_status,
       COALESCE(MAX(cs.step_id) FILTER (WHERE cs.state = 'active'), '') AS current_step
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
LEFT JOIN case_steps cs ON cs.case_id = c.id
WHERE c.tenant_id = $1
`
	args := []interface{}{tenantID}
	idx := 2

	if len(filter.Statuses) > 0 {
		query += fmt.Sprintf(" AND c.status = ANY($%d)", idx)
		args = append(args, pqStringArray(filter.Statuses))
		idx++
	}
	if filter.CaseType != "" {
		query += fmt.Sprintf(" AND ct.name = $%d", idx)
		args = append(args, filter.CaseType)
		idx++
	}
	if filter.AssignedNone {
		query += " AND c.assigned_to IS NULL"
	} else if filter.AssignedTo != nil {
		query += fmt.Sprintf(" AND c.assigned_to = $%d", idx)
		args = append(args, *filter.AssignedTo)
		idx++
	}
	if filter.OlderThanDays != nil {
		query += fmt.Sprintf(" AND c.created_at < now() - ($%d || ' days')::interval", idx)
		args = append(args, strconv.Itoa(*filter.OlderThanDays))
		idx++
	}
	if filter.Priority != nil {
		query += fmt.Sprintf(" AND c.priority >= $%d", idx)
		args = append(args, *filter.Priority)
		idx++
	}
	if filter.CreatedAfter != nil {
		query += fmt.Sprintf(" AND c.created_at >= $%d", idx)
		args = append(args, *filter.CreatedAfter)
		idx++
	}
	if filter.CreatedBefore != nil {
		query += fmt.Sprintf(" AND c.created_at <= $%d", idx)
		args = append(args, *filter.CreatedBefore)
		idx++
	}

	query += " GROUP BY c.id, ct.name"
	if filter.SLAStatus != "" {
		query += fmt.Sprintf(" HAVING COALESCE(CASE WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) IS NULL THEN 'n/a' WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) < now() THEN 'breached' WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) < now() + interval '24 hour' THEN 'warning' ELSE 'on_track' END, 'n/a') = $%d", idx)
		args = append(args, filter.SLAStatus)
		idx++
	}

	query += " ORDER BY " + safeDashboardSort(filter.SortBy, filter.SortDir)
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]DashboardRow, 0)
	for rows.Next() {
		var row DashboardRow
		if err := rows.Scan(&row.CaseID, &row.CaseNumber, &row.CaseType, &row.Status, &row.AssignedTo, &row.Priority, &row.CreatedAt, &row.UpdatedAt, &row.SLAStatus, &row.CurrentStep); err != nil {
			return nil, fmt.Errorf("scan dashboard row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func safeCaseSort(sortBy, sortDir string) string {
	allowed := map[string]string{
		"created_at":  "c.created_at",
		"updated_at":  "c.updated_at",
		"priority":    "c.priority",
		"due_at":      "c.due_at",
		"case_number": "c.case_number",
		"status":      "c.status",
	}
	col, ok := allowed[sortBy]
	if !ok {
		col = "c.updated_at"
	}
	dir := strings.ToUpper(sortDir)
	if dir != "ASC" {
		dir = "DESC"
	}
	return col + " " + dir
}

func safeDashboardSort(sortBy, sortDir string) string {
	allowed := map[string]string{
		"case_number": "c.case_number",
		"created_at":  "c.created_at",
		"updated_at":  "c.updated_at",
		"priority":    "c.priority",
		"status":      "c.status",
	}
	col, ok := allowed[sortBy]
	if !ok {
		col = "c.updated_at"
	}
	dir := strings.ToUpper(sortDir)
	if dir != "ASC" {
		dir = "DESC"
	}
	return col + " " + dir
}

func normalizePage(page, perPage int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}
