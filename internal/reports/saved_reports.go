package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Service) SaveReport(ctx context.Context, tenantID, principalID uuid.UUID, req SaveReportRequest) (SavedReport, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.QuerySQL) == "" {
		return SavedReport{}, fmt.Errorf("name and query_sql are required")
	}
	if err := s.inspector.Validate(req.QuerySQL); err != nil {
		return SavedReport{}, err
	}
	colsRaw, _ := json.Marshal(req.Columns)
	var out SavedReport
	err := s.db.QueryRowContext(ctx, `
INSERT INTO saved_reports (
    tenant_id, created_by, name, description, original_question, query_sql, visualisation, columns
) VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8::jsonb)
RETURNING id, tenant_id, created_by, name, COALESCE(description,''), COALESCE(original_question,''), query_sql, visualisation, columns, COALESCE(parameters, '{}'::jsonb), is_published, pinned, COALESCE(schedule,''), COALESCE(recipients, '[]'::jsonb), created_at, updated_at, last_run_at
`, tenantID, principalID, req.Name, req.Description, req.OriginalQuestion, req.QuerySQL, normalizeVisualisation(req.Visualisation), string(colsRaw)).Scan(
		&out.ID, &out.TenantID, &out.CreatedBy, &out.Name, &out.Description, &out.OriginalQuestion,
		&out.QuerySQL, &out.Visualisation, &colsRaw, &out.Parameters, &out.IsPublished, &out.Pinned,
		&out.Schedule, &out.Recipients, &out.CreatedAt, &out.UpdatedAt, &out.LastRunAt,
	)
	if err != nil {
		return SavedReport{}, fmt.Errorf("insert saved report: %w", err)
	}
	_ = json.Unmarshal(colsRaw, &out.Columns)
	return out, nil
}

func (s *Service) ListReports(ctx context.Context, tenantID, principalID uuid.UUID, scope string, isAdmin bool) ([]SavedReport, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "mine"
	}
	query := `
SELECT id, tenant_id, created_by, name, COALESCE(description,''), COALESCE(original_question,''), query_sql, visualisation, columns, COALESCE(parameters, '{}'::jsonb), is_published, pinned, COALESCE(schedule,''), COALESCE(recipients, '[]'::jsonb), created_at, updated_at, last_run_at
FROM saved_reports
WHERE tenant_id = $1
`
	args := []any{tenantID}
	switch scope {
	case "mine":
		args = append(args, principalID)
		query += fmt.Sprintf(" AND created_by = $%d", len(args))
	case "published":
		query += " AND is_published = true"
	case "all":
		if !isAdmin {
			return nil, fmt.Errorf("forbidden")
		}
	default:
		return nil, fmt.Errorf("invalid scope")
	}
	query += " ORDER BY pinned DESC, updated_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query saved reports: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]SavedReport, 0)
	for rows.Next() {
		var (
			item SavedReport
			raw  []byte
		)
		if err := rows.Scan(&item.ID, &item.TenantID, &item.CreatedBy, &item.Name, &item.Description, &item.OriginalQuestion, &item.QuerySQL, &item.Visualisation, &raw, &item.Parameters, &item.IsPublished, &item.Pinned, &item.Schedule, &item.Recipients, &item.CreatedAt, &item.UpdatedAt, &item.LastRunAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Columns)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) GetReport(ctx context.Context, tenantID, reportID uuid.UUID) (SavedReport, error) {
	var (
		item SavedReport
		raw  []byte
	)
	err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, created_by, name, COALESCE(description,''), COALESCE(original_question,''), query_sql, visualisation, columns, COALESCE(parameters, '{}'::jsonb), is_published, pinned, COALESCE(schedule,''), COALESCE(recipients, '[]'::jsonb), created_at, updated_at, last_run_at
FROM saved_reports
WHERE tenant_id = $1 AND id = $2
`, tenantID, reportID).Scan(&item.ID, &item.TenantID, &item.CreatedBy, &item.Name, &item.Description, &item.OriginalQuestion, &item.QuerySQL, &item.Visualisation, &raw, &item.Parameters, &item.IsPublished, &item.Pinned, &item.Schedule, &item.Recipients, &item.CreatedAt, &item.UpdatedAt, &item.LastRunAt)
	if err != nil {
		return SavedReport{}, err
	}
	_ = json.Unmarshal(raw, &item.Columns)
	return item, nil
}

func (s *Service) RunReport(ctx context.Context, tenantID uuid.UUID, reportID uuid.UUID) (AskResponse, error) {
	report, err := s.GetReport(ctx, tenantID, reportID)
	if err != nil {
		return AskResponse{}, err
	}
	rows, cols, err := s.ExecuteSQL(ctx, tenantID, report.QuerySQL)
	if err != nil {
		return AskResponse{}, err
	}
	if len(report.Columns) == 0 {
		report.Columns = columnsFromNames(cols)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE saved_reports SET last_run_at = now(), updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenantID, reportID)
	return AskResponse{
		Title:         report.Name,
		SQL:           report.QuerySQL,
		Visualisation: report.Visualisation,
		Columns:       report.Columns,
		Rows:          rows,
		RowCount:      len(rows),
	}, nil
}

func (s *Service) UpdateReport(ctx context.Context, tenantID, principalID, reportID uuid.UUID, req UpdateReportRequest, isAdmin bool) (SavedReport, error) {
	report, err := s.GetReport(ctx, tenantID, reportID)
	if err != nil {
		return SavedReport{}, err
	}
	if report.CreatedBy != principalID && !isAdmin {
		return SavedReport{}, fmt.Errorf("forbidden")
	}
	columns := report.Columns
	if req.Columns != nil {
		columns = *req.Columns
	}
	colsRaw, _ := json.Marshal(columns)
	name := report.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	desc := report.Description
	if req.Description != nil {
		desc = *req.Description
	}
	vis := report.Visualisation
	if req.Visualisation != nil {
		vis = normalizeVisualisation(*req.Visualisation)
	}
	pub := report.IsPublished
	if req.IsPublished != nil {
		pub = *req.IsPublished
	}
	pinned := report.Pinned
	if req.Pinned != nil {
		pinned = *req.Pinned
	}
	schedule := report.Schedule
	if req.Schedule != nil {
		schedule = strings.TrimSpace(*req.Schedule)
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE saved_reports
SET name = $3,
    description = NULLIF($4, ''),
    visualisation = $5,
    columns = $6::jsonb,
    is_published = $7,
    pinned = $8,
    schedule = NULLIF($9, ''),
    updated_at = now()
WHERE tenant_id = $1 AND id = $2
`, tenantID, reportID, name, desc, vis, string(colsRaw), pub, pinned, schedule); err != nil {
		return SavedReport{}, err
	}
	return s.GetReport(ctx, tenantID, reportID)
}

func (s *Service) DeleteReport(ctx context.Context, tenantID, principalID, reportID uuid.UUID, isAdmin bool) error {
	report, err := s.GetReport(ctx, tenantID, reportID)
	if err != nil {
		return err
	}
	if report.CreatedBy != principalID && !isAdmin {
		return fmt.Errorf("forbidden")
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM saved_reports WHERE tenant_id = $1 AND id = $2`, tenantID, reportID)
	return err
}
