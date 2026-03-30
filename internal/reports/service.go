package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/agents"
)

const (
	defaultStatementTimeout = 5 * time.Second
	defaultRefreshInterval  = 5 * time.Minute
	defaultScheduleInterval = 1 * time.Minute
	maxRows                 = 10000
	maxResultBytes          = 5 * 1024 * 1024
)

type LLM interface {
	ChatCompletion(ctx context.Context, messages []agents.Message, responseFormat *agents.ResponseFormat) (*agents.ChatResponse, error)
}

type Service struct {
	db                *sql.DB
	reporterDB        *sql.DB
	llm               LLM
	inspector         SQLInspector
	logger            *log.Logger
	statementTimeout  time.Duration
	refreshInterval   time.Duration
	scheduleInterval  time.Duration
	now               func() time.Time
	sendScheduleEmail func(ctx context.Context, tenantID uuid.UUID, report SavedReport, rows []map[string]any, columns []string) error
}

func NewService(db *sql.DB, llm LLM) *Service {
	s := &Service{
		db:               db,
		reporterDB:       db,
		llm:              llm,
		inspector:        NewSQLInspector(),
		logger:           log.Default(),
		statementTimeout: defaultStatementTimeout,
		refreshInterval:  defaultRefreshInterval,
		scheduleInterval: defaultScheduleInterval,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	s.sendScheduleEmail = s.defaultScheduleEmail
	return s
}

func (s *Service) SetScheduleEmailSender(fn func(ctx context.Context, tenantID uuid.UUID, report SavedReport, rows []map[string]any, columns []string) error) {
	if fn == nil {
		s.sendScheduleEmail = s.defaultScheduleEmail
		return
	}
	s.sendScheduleEmail = fn
}

func (s *Service) StartViewRefreshTicker(ctx context.Context) {
	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.RefreshMaterializedViews(ctx); err != nil {
				s.logger.Printf("reports: refresh materialized views failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) StartScheduleTicker(ctx context.Context) {
	ticker := time.NewTicker(s.scheduleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.RunDueScheduledReports(ctx); err != nil {
				s.logger.Printf("reports: run due scheduled reports failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) RefreshMaterializedViews(ctx context.Context) error {
	for _, view := range []string{"mv_report_cases", "mv_report_steps", "mv_report_tasks"} {
		if _, err := s.db.ExecContext(ctx, `REFRESH MATERIALIZED VIEW `+view); err != nil {
			return fmt.Errorf("refresh %s: %w", view, err)
		}
	}
	return nil
}

func (s *Service) Ask(ctx context.Context, tenantID uuid.UUID, question string) (AskResponse, error) {
	if strings.TrimSpace(question) == "" {
		return AskResponse{}, fmt.Errorf("question is required")
	}
	viewSchemas, err := s.LoadViewSchemas(ctx)
	if err != nil {
		return AskResponse{}, err
	}
	prompt := BuildPrompt(viewSchemas, question)
	answer, err := s.generateLLMAnswer(ctx, prompt)
	if err != nil {
		return AskResponse{}, err
	}
	rows, cols, err := s.ExecuteSQL(ctx, tenantID, answer.SQL)
	if err != nil {
		return AskResponse{}, err
	}
	if len(answer.Columns) == 0 {
		answer.Columns = columnsFromNames(cols)
	}
	return AskResponse{
		Title:         answer.Title,
		SQL:           answer.SQL,
		Visualisation: normalizeVisualisation(answer.Visualisation),
		Columns:       answer.Columns,
		Rows:          rows,
		RowCount:      len(rows),
	}, nil
}

func (s *Service) ExecuteSQL(ctx context.Context, tenantID uuid.UUID, sqlText string) ([]map[string]any, []string, error) {
	scopedSQL, err := s.inspector.ScopeToTenant(sqlText)
	if err != nil {
		return nil, nil, err
	}
	tx, err := s.reporterDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, fmt.Errorf("begin reporting tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL statement_timeout = '5s'`); err != nil {
		return nil, nil, fmt.Errorf("set statement timeout: %w", err)
	}
	// Read-only reporter role is defense in depth for generated SQL.
	_, _ = tx.ExecContext(ctx, `SET LOCAL ROLE aceryx_reporter`)

	rows, err := tx.QueryContext(ctx, scopedSQL, tenantID, maxRows)
	if err != nil {
		s.logger.Printf("reports query failed sql=%q err=%v", scopedSQL, err)
		return nil, nil, fmt.Errorf("couldn't run that query; try rephrasing your question or being more specific")
	}
	defer func() { _ = rows.Close() }()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("load result columns: %w", err)
	}
	out := make([]map[string]any, 0)
	totalBytes := 0
	for rows.Next() {
		values := make([]any, len(columnNames))
		scans := make([]any, len(columnNames))
		for i := range values {
			scans[i] = &values[i]
		}
		if err := rows.Scan(scans...); err != nil {
			return nil, nil, fmt.Errorf("scan report row: %w", err)
		}
		item := map[string]any{}
		for i, name := range columnNames {
			item[name] = normalizeSQLValue(values[i])
		}
		out = append(out, item)
		b, _ := json.Marshal(item)
		totalBytes += len(b)
		if totalBytes > maxResultBytes {
			return nil, nil, fmt.Errorf("result too large; narrow your query")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate report rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit reporting tx: %w", err)
	}
	return out, columnNames, nil
}

func (s *Service) LoadViewSchemas(ctx context.Context) ([]ViewSchema, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT view_name, description, columns
FROM report_view_schemas
ORDER BY view_name
`)
	if err != nil {
		return nil, fmt.Errorf("query report view schemas: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ViewSchema, 0)
	for rows.Next() {
		var (
			item ViewSchema
			raw  []byte
		)
		if err := rows.Scan(&item.ViewName, &item.Description, &raw); err != nil {
			return nil, fmt.Errorf("scan report view schema: %w", err)
		}
		if err := json.Unmarshal(raw, &item.Columns); err != nil {
			return nil, fmt.Errorf("decode report view schema columns: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func BuildPrompt(views []ViewSchema, question string) string {
	var b strings.Builder
	b.WriteString("You are a SQL query generator for a business workflow reporting system.\n\n")
	b.WriteString("Available views and their columns:\n")
	for _, v := range views {
		b.WriteString("View: " + v.ViewName + "\n")
		b.WriteString("Description: " + v.Description + "\n")
		b.WriteString("Columns:\n")
		for _, c := range v.Columns {
			b.WriteString(fmt.Sprintf("  - %s (%s): %s\n", c.Name, c.Type, c.Description))
		}
		b.WriteString("\n")
	}
	b.WriteString(`The user's question: "` + question + `"` + "\n\n")
	b.WriteString(`Respond with ONLY a JSON object (no markdown, no explanation):
{
  "sql": "SELECT ... FROM mv_report_cases WHERE ...",
  "title": "Human-readable title for this report",
  "visualisation": "table|bar|line|pie|number",
  "columns": [
    { "key": "column_name", "label": "Display Label", "role": "dimension|measure|info" }
  ]
}

Rules:
- Only SELECT statements. No INSERT, UPDATE, DELETE, DROP, or DDL.
- Only query the views listed above. No other tables.
- Do not include a tenant_id filter — it is applied automatically.
- Use standard Postgres SQL.
- For "number" visualisation, return exactly one row with one numeric column.`)
	return b.String()
}

func (s *Service) generateLLMAnswer(ctx context.Context, prompt string) (LLMAnswer, error) {
	if s.llm == nil {
		return LLMAnswer{}, fmt.Errorf("reporting LLM is not configured")
	}
	resp, err := s.llm.ChatCompletion(ctx, []agents.Message{
		{Role: "system", Content: "You return JSON only."},
		{Role: "user", Content: prompt},
	}, &agents.ResponseFormat{Type: "json_object"})
	if err != nil {
		return LLMAnswer{}, fmt.Errorf("report generation failed: %w", err)
	}
	var answer LLMAnswer
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &answer); err != nil {
		return LLMAnswer{}, fmt.Errorf("couldn't understand that report output; try rephrasing your question")
	}
	if strings.TrimSpace(answer.SQL) == "" {
		return LLMAnswer{}, fmt.Errorf("couldn't run that query; try rephrasing your question or being more specific")
	}
	if answer.Title == "" {
		answer.Title = "Report"
	}
	answer.Visualisation = normalizeVisualisation(answer.Visualisation)
	return answer, nil
}

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
RETURNING id, tenant_id, created_by, name, COALESCE(description,''), COALESCE(original_question,''), query_sql, visualisation, columns, parameters, is_published, pinned, COALESCE(schedule,''), recipients, created_at, updated_at, last_run_at
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
SELECT id, tenant_id, created_by, name, COALESCE(description,''), COALESCE(original_question,''), query_sql, visualisation, columns, parameters, is_published, pinned, COALESCE(schedule,''), recipients, created_at, updated_at, last_run_at
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
SELECT id, tenant_id, created_by, name, COALESCE(description,''), COALESCE(original_question,''), query_sql, visualisation, columns, parameters, is_published, pinned, COALESCE(schedule,''), recipients, created_at, updated_at, last_run_at
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

func (s *Service) RunDueScheduledReports(ctx context.Context) error {
	for {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		var (
			reportID uuid.UUID
			tenantID uuid.UUID
		)
		err = tx.QueryRowContext(ctx, `
SELECT id, tenant_id
FROM saved_reports
WHERE schedule IN ('daily','weekly','monthly')
  AND (
      last_run_at IS NULL
      OR (schedule = 'daily' AND last_run_at < now() - interval '1 day')
      OR (schedule = 'weekly' AND last_run_at < now() - interval '7 day')
      OR (schedule = 'monthly' AND last_run_at < now() - interval '1 month')
  )
ORDER BY COALESCE(last_run_at, to_timestamp(0))
FOR UPDATE SKIP LOCKED
LIMIT 1
`).Scan(&reportID, &tenantID)
		if err != nil {
			_ = tx.Rollback()
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE saved_reports SET last_run_at = now(), updated_at = now() WHERE id = $1`, reportID); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}

		report, err := s.GetReport(ctx, tenantID, reportID)
		if err != nil {
			s.logger.Printf("reports schedule: load report failed id=%s err=%v", reportID, err)
			continue
		}
		rows, cols, err := s.ExecuteSQL(ctx, tenantID, report.QuerySQL)
		if err != nil {
			s.logger.Printf("reports schedule: execute failed id=%s err=%v", reportID, err)
			continue
		}
		_ = s.sendScheduleEmail(ctx, tenantID, report, rows, cols)
	}
}

func (s *Service) defaultScheduleEmail(_ context.Context, _ uuid.UUID, report SavedReport, _ []map[string]any, _ []string) error {
	s.logger.Printf("scheduled report executed: %s (%s)", report.Name, report.ID)
	return nil
}

func normalizeVisualisation(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "bar", "line", "pie", "number":
		return v
	default:
		return "table"
	}
}

func normalizeSQLValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	default:
		return x
	}
}

func columnsFromNames(names []string) []ReportColumn {
	out := make([]ReportColumn, 0, len(names))
	for i, name := range names {
		role := "info"
		switch i {
		case 0:
			role = "dimension"
		case 1:
			role = "measure"
		}
		out = append(out, ReportColumn{Key: name, Label: labelFromKey(name), Role: role})
	}
	return out
}

func labelFromKey(key string) string {
	key = strings.TrimSpace(strings.ReplaceAll(key, "_", " "))
	if key == "" {
		return ""
	}
	parts := strings.Fields(key)
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

var errFriendly = regexp.MustCompile(`(?i)(syntax|column|function|relation|timeout|permission|denied|error)`)

func FriendlyError(err error) string {
	if err == nil {
		return ""
	}
	if errFriendly.MatchString(err.Error()) {
		return "I couldn't run that query. Try rephrasing your question or being more specific."
	}
	return err.Error()
}
