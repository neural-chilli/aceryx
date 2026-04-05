package drivers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

var (
	reSelectStmt = regexp.MustCompile(`(?is)^\s*select\b`)
	reHasLimit   = regexp.MustCompile(`(?is)\blimit\s+\d+`)
	rePgParams   = regexp.MustCompile(`\$[0-9]+`)
)

type WriteAuthorizer interface {
	Require(ctx context.Context, permission string) error
}

type QueryExecutor struct {
	poolManager     *PoolManager
	registry        *DriverRegistry
	writeAuthorizer WriteAuthorizer
}

type QueryRequest struct {
	TenantID string
	DriverID string
	Config   DBConfig
	Query    string
	Params   []any
	ReadOnly bool
	Timeout  time.Duration
	RowLimit int
}

type QueryResult struct {
	Columns    []string                 `json:"columns"`
	Rows       []map[string]interface{} `json:"rows"`
	RowCount   int                      `json:"row_count"`
	Truncated  bool                     `json:"truncated"`
	DurationMS int                      `json:"duration_ms"`
}

func NewQueryExecutor(registry *DriverRegistry, poolManager *PoolManager, authorizer WriteAuthorizer) *QueryExecutor {
	if poolManager == nil {
		poolManager = NewPoolManager()
	}
	return &QueryExecutor{poolManager: poolManager, registry: registry, writeAuthorizer: authorizer}
}

func (qe *QueryExecutor) Execute(ctx context.Context, req QueryRequest) (QueryResult, error) {
	if qe == nil || qe.registry == nil {
		return QueryResult{}, fmt.Errorf("query executor not configured")
	}
	driver, err := qe.registry.GetDB(strings.TrimSpace(req.DriverID))
	if err != nil {
		return QueryResult{}, err
	}
	if strings.TrimSpace(req.Query) == "" {
		return QueryResult{}, fmt.Errorf("query is required")
	}

	cfg := withDBDefaults(req.Config)
	readOnly := req.ReadOnly
	if !req.ReadOnly && isSelectQuery(req.Query) {
		// Safe default: SELECT runs read-only unless caller explicitly asks otherwise.
		readOnly = true
	}
	if !readOnly {
		if qe.writeAuthorizer == nil {
			return QueryResult{}, fmt.Errorf("insufficient permissions: connectors:db:write required")
		}
		if err := qe.writeAuthorizer.Require(ctx, "connectors:db:write"); err != nil {
			return QueryResult{}, fmt.Errorf("insufficient permissions: connectors:db:write required")
		}
	}

	rowLimit := req.RowLimit
	if rowLimit <= 0 {
		rowLimit = cfg.RowLimit
	}
	if rowLimit <= 0 {
		rowLimit = 10000
	}

	timeout := time.Duration(cfg.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if req.Timeout > 0 && req.Timeout < timeout {
		timeout = req.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	db, err := qe.poolManager.GetOrCreate(ctx, req.TenantID, driver, cfg)
	if err != nil {
		return QueryResult{}, err
	}

	query := strings.TrimSpace(req.Query)
	if driver.ID() == "mysql" {
		query = toMySQLParams(query)
	}
	if isSelectQuery(query) && !reHasLimit.MatchString(query) {
		query = fmt.Sprintf("SELECT * FROM (%s) AS aceryx_q LIMIT %d", strings.TrimSuffix(query, ";"), rowLimit)
	}

	start := time.Now()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: readOnly})
	if err != nil {
		return QueryResult{}, fmt.Errorf("begin query tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, query, req.Params...)
	if err != nil {
		return QueryResult{}, fmt.Errorf("execute query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return QueryResult{}, fmt.Errorf("read columns: %w", err)
	}
	out := make([]map[string]interface{}, 0)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return QueryResult{}, fmt.Errorf("scan row: %w", err)
		}
		row := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			row[c] = normalizeValue(vals[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return QueryResult{}, fmt.Errorf("iterate rows: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return QueryResult{}, fmt.Errorf("commit query tx: %w", err)
	}

	truncated := len(out) == rowLimit && isSelectQuery(req.Query)
	if truncated {
		slog.WarnContext(ctx, "query result truncated", "row_limit", rowLimit, "driver_id", req.DriverID)
	}
	return QueryResult{
		Columns:    cols,
		Rows:       out,
		RowCount:   len(out),
		Truncated:  truncated,
		DurationMS: int(time.Since(start).Milliseconds()),
	}, nil
}

func isSelectQuery(query string) bool {
	return reSelectStmt.MatchString(strings.TrimSpace(query))
}

func toMySQLParams(query string) string {
	return rePgParams.ReplaceAllString(query, "?")
}

func normalizeValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return append([]byte(nil), t...)
	default:
		return t
	}
}
