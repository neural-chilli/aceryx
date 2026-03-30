package reports

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

var ApprovedViews = []string{"mv_report_cases", "mv_report_steps", "mv_report_tasks"}

var AllowedFunctions = map[string]struct{}{
	"count":      {},
	"sum":        {},
	"avg":        {},
	"min":        {},
	"max":        {},
	"extract":    {},
	"date_trunc": {},
	"coalesce":   {},
	"round":      {},
}

type ViewSchemaColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ViewSchema struct {
	ViewName    string             `json:"view_name"`
	Description string             `json:"description"`
	Columns     []ViewSchemaColumn `json:"columns"`
}

type ReportColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Role  string `json:"role"`
}

type LLMAnswer struct {
	SQL           string         `json:"sql"`
	Title         string         `json:"title"`
	Visualisation string         `json:"visualisation"`
	Columns       []ReportColumn `json:"columns"`
}

type AskResponse struct {
	Title         string           `json:"title"`
	SQL           string           `json:"sql"`
	Visualisation string           `json:"visualisation"`
	Columns       []ReportColumn   `json:"columns"`
	Rows          []map[string]any `json:"rows"`
	RowCount      int              `json:"row_count"`
}

type SavedReport struct {
	ID               uuid.UUID       `json:"id"`
	TenantID         uuid.UUID       `json:"tenant_id"`
	CreatedBy        uuid.UUID       `json:"created_by"`
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	OriginalQuestion string          `json:"original_question,omitempty"`
	QuerySQL         string          `json:"query_sql"`
	Visualisation    string          `json:"visualisation"`
	Columns          []ReportColumn  `json:"columns"`
	Parameters       json.RawMessage `json:"parameters,omitempty"`
	IsPublished      bool            `json:"is_published"`
	Pinned           bool            `json:"pinned"`
	Schedule         string          `json:"schedule,omitempty"`
	Recipients       json.RawMessage `json:"recipients,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	LastRunAt        *time.Time      `json:"last_run_at,omitempty"`
}

type SaveReportRequest struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	OriginalQuestion string         `json:"original_question"`
	QuerySQL         string         `json:"query_sql"`
	Visualisation    string         `json:"visualisation"`
	Columns          []ReportColumn `json:"columns"`
}

type UpdateReportRequest struct {
	Name          *string         `json:"name,omitempty"`
	Description   *string         `json:"description,omitempty"`
	Visualisation *string         `json:"visualisation,omitempty"`
	Columns       *[]ReportColumn `json:"columns,omitempty"`
	IsPublished   *bool           `json:"is_published,omitempty"`
	Pinned        *bool           `json:"pinned,omitempty"`
	Schedule      *string         `json:"schedule,omitempty"`
}
