package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/agents"
)

type LLM interface {
	ChatCompletion(ctx context.Context, messages []agents.Message, responseFormat *agents.ResponseFormat) (*agents.ChatResponse, error)
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
	if err := s.RefreshMaterializedViews(ctx); err != nil {
		return nil, nil, fmt.Errorf("refresh reporting views: %w", err)
	}
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
