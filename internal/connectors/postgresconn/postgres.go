package postgresconn

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/neural-chilli/aceryx/internal/connectors"
)

var identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Connector struct{}

func New() *Connector { return &Connector{} }

func (c *Connector) Meta() connectors.ConnectorMeta {
	return connectors.ConnectorMeta{
		Key:         "postgres",
		Name:        "PostgreSQL",
		Description: "Run SQL templates or structured CRUD operations against PostgreSQL",
		Version:     "v1",
		Icon:        "pi pi-database",
	}
}

func (c *Connector) Auth() connectors.AuthSpec {
	return connectors.AuthSpec{
		Type: "basic",
		Fields: []connectors.AuthField{
			{Key: "dsn", Label: "DSN", Type: "string", Required: false},
			{Key: "host", Label: "Host", Type: "string", Required: false},
			{Key: "port", Label: "Port", Type: "number", Required: false},
			{Key: "database", Label: "Database", Type: "string", Required: false},
			{Key: "user", Label: "User", Type: "string", Required: false},
			{Key: "password", Label: "Password", Type: "password", Required: false},
			{Key: "sslmode", Label: "SSL Mode", Type: "string", Required: false},
		},
	}
}

func (c *Connector) Triggers() []connectors.TriggerSpec { return nil }

func (c *Connector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{
		{Key: "select", Name: "Select", Description: "Structured SELECT query", InputSchema: schemaSelect(), OutputSchema: schemaRows(), Execute: c.selectAction},
		{Key: "insert", Name: "Insert", Description: "Structured INSERT", InputSchema: schemaInsert(), OutputSchema: schemaMutation(), Execute: c.insertAction},
		{Key: "update", Name: "Update", Description: "Structured UPDATE", InputSchema: schemaUpdate(), OutputSchema: schemaMutation(), Execute: c.updateAction},
		{Key: "delete", Name: "Delete", Description: "Structured DELETE", InputSchema: schemaDelete(), OutputSchema: schemaMutation(), Execute: c.deleteAction},
		{Key: "upsert", Name: "Upsert", Description: "Structured INSERT ... ON CONFLICT", InputSchema: schemaUpsert(), OutputSchema: schemaMutation(), Execute: c.upsertAction},
		{Key: "query_template", Name: "Query Template", Description: "Parameterized SQL query template", InputSchema: schemaQueryTemplate(), OutputSchema: schemaRows(), Execute: c.queryTemplateAction},
		{Key: "exec_template", Name: "Exec Template", Description: "Parameterized SQL command template", InputSchema: schemaQueryTemplate(), OutputSchema: schemaMutation(), Execute: c.execTemplateAction},
	}
}

func (c *Connector) selectAction(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	db, closeFn, err := openDB(ctx, auth)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	table, err := readRequiredIdentifier(input, "table")
	if err != nil {
		return nil, err
	}

	columns, err := readColumnList(input["columns"], []string{"*"})
	if err != nil {
		return nil, err
	}
	selectClause := "*"
	if len(columns) > 0 && !(len(columns) == 1 && columns[0] == "*") {
		quoted := make([]string, 0, len(columns))
		for _, col := range columns {
			quoted = append(quoted, quoteIdent(col))
		}
		selectClause = strings.Join(quoted, ", ")
	}

	query := "SELECT " + selectClause + " FROM " + quoteIdent(table)
	whereSQL, args, err := buildWhereSQL(input["where"], 1)
	if err != nil {
		return nil, err
	}
	if whereSQL != "" {
		query += " WHERE " + whereSQL
	}

	orderBy, err := buildOrderBy(input["order_by"])
	if err != nil {
		return nil, err
	}
	if orderBy != "" {
		query += " ORDER BY " + orderBy
	}

	if limit := readInt(input, "limit", 0); limit > 0 {
		query += " LIMIT " + strconv.Itoa(limit)
	}
	if offset := readInt(input, "offset", 0); offset > 0 {
		query += " OFFSET " + strconv.Itoa(offset)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres select failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{"rows": items, "row_count": len(items)}, nil
}

func (c *Connector) insertAction(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	db, closeFn, err := openDB(ctx, auth)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	table, err := readRequiredIdentifier(input, "table")
	if err != nil {
		return nil, err
	}
	values, err := readObject(input, "values")
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("values is required")
	}

	keys := sortedKeys(values)
	cols := make([]string, 0, len(keys))
	ph := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for i, key := range keys {
		if !isIdentifier(key) {
			return nil, fmt.Errorf("invalid column name: %s", key)
		}
		cols = append(cols, quoteIdent(key))
		ph = append(ph, "$"+strconv.Itoa(i+1))
		args = append(args, values[key])
	}

	query := "INSERT INTO " + quoteIdent(table) + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(ph, ", ") + ")"
	returning, err := readColumnList(input["returning"], nil)
	if err != nil {
		return nil, err
	}
	if len(returning) > 0 {
		query += " RETURNING " + joinQuoted(returning)
		rows, qerr := db.QueryContext(ctx, query, args...)
		if qerr != nil {
			return nil, fmt.Errorf("postgres insert failed: %w", qerr)
		}
		defer func() { _ = rows.Close() }()
		items, serr := scanRows(rows)
		if serr != nil {
			return nil, serr
		}
		first := map[string]any{}
		if len(items) > 0 {
			first = items[0]
		}
		return map[string]any{"rows_affected": len(items), "row": first, "rows": items}, nil
	}

	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres insert failed: %w", err)
	}
	affected, _ := res.RowsAffected()
	return map[string]any{"rows_affected": affected}, nil
}

func (c *Connector) updateAction(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	db, closeFn, err := openDB(ctx, auth)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	table, err := readRequiredIdentifier(input, "table")
	if err != nil {
		return nil, err
	}
	setValues, err := readObject(input, "set")
	if err != nil {
		return nil, err
	}
	if len(setValues) == 0 {
		return nil, fmt.Errorf("set is required")
	}
	setKeys := sortedKeys(setValues)
	setClauses := make([]string, 0, len(setKeys))
	args := make([]any, 0, len(setKeys)+4)
	for idx, key := range setKeys {
		if !isIdentifier(key) {
			return nil, fmt.Errorf("invalid column name: %s", key)
		}
		setClauses = append(setClauses, quoteIdent(key)+" = $"+strconv.Itoa(idx+1))
		args = append(args, setValues[key])
	}

	whereSQL, whereArgs, err := buildWhereSQL(input["where"], len(args)+1)
	if err != nil {
		return nil, err
	}
	allowAll := readBool(input, "allow_all", false)
	if whereSQL == "" && !allowAll {
		return nil, fmt.Errorf("where is required unless allow_all is true")
	}
	args = append(args, whereArgs...)

	query := "UPDATE " + quoteIdent(table) + " SET " + strings.Join(setClauses, ", ")
	if whereSQL != "" {
		query += " WHERE " + whereSQL
	}

	returning, err := readColumnList(input["returning"], nil)
	if err != nil {
		return nil, err
	}
	if len(returning) > 0 {
		query += " RETURNING " + joinQuoted(returning)
		rows, qerr := db.QueryContext(ctx, query, args...)
		if qerr != nil {
			return nil, fmt.Errorf("postgres update failed: %w", qerr)
		}
		defer func() { _ = rows.Close() }()
		items, serr := scanRows(rows)
		if serr != nil {
			return nil, serr
		}
		return map[string]any{"rows_affected": len(items), "rows": items}, nil
	}

	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres update failed: %w", err)
	}
	affected, _ := res.RowsAffected()
	return map[string]any{"rows_affected": affected}, nil
}

func (c *Connector) deleteAction(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	db, closeFn, err := openDB(ctx, auth)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	table, err := readRequiredIdentifier(input, "table")
	if err != nil {
		return nil, err
	}
	whereSQL, whereArgs, err := buildWhereSQL(input["where"], 1)
	if err != nil {
		return nil, err
	}
	allowAll := readBool(input, "allow_all", false)
	if whereSQL == "" && !allowAll {
		return nil, fmt.Errorf("where is required unless allow_all is true")
	}

	query := "DELETE FROM " + quoteIdent(table)
	if whereSQL != "" {
		query += " WHERE " + whereSQL
	}

	returning, err := readColumnList(input["returning"], nil)
	if err != nil {
		return nil, err
	}
	if len(returning) > 0 {
		query += " RETURNING " + joinQuoted(returning)
		rows, qerr := db.QueryContext(ctx, query, whereArgs...)
		if qerr != nil {
			return nil, fmt.Errorf("postgres delete failed: %w", qerr)
		}
		defer func() { _ = rows.Close() }()
		items, serr := scanRows(rows)
		if serr != nil {
			return nil, serr
		}
		return map[string]any{"rows_affected": len(items), "rows": items}, nil
	}

	res, err := db.ExecContext(ctx, query, whereArgs...)
	if err != nil {
		return nil, fmt.Errorf("postgres delete failed: %w", err)
	}
	affected, _ := res.RowsAffected()
	return map[string]any{"rows_affected": affected}, nil
}

func (c *Connector) upsertAction(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	db, closeFn, err := openDB(ctx, auth)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	table, err := readRequiredIdentifier(input, "table")
	if err != nil {
		return nil, err
	}
	values, err := readObject(input, "values")
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("values is required")
	}
	conflictColumns, err := readColumnList(input["conflict_columns"], nil)
	if err != nil {
		return nil, err
	}
	if len(conflictColumns) == 0 {
		return nil, fmt.Errorf("conflict_columns is required")
	}

	keys := sortedKeys(values)
	cols := make([]string, 0, len(keys))
	ph := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for i, key := range keys {
		if !isIdentifier(key) {
			return nil, fmt.Errorf("invalid column name: %s", key)
		}
		cols = append(cols, quoteIdent(key))
		ph = append(ph, "$"+strconv.Itoa(i+1))
		args = append(args, values[key])
	}

	query := "INSERT INTO " + quoteIdent(table) + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(ph, ", ") + ")"
	query += " ON CONFLICT (" + joinQuoted(conflictColumns) + ")"

	updateColumns, err := readColumnList(input["update_columns"], nil)
	if err != nil {
		return nil, err
	}
	if len(updateColumns) == 0 {
		conflictSet := make(map[string]struct{}, len(conflictColumns))
		for _, col := range conflictColumns {
			conflictSet[col] = struct{}{}
		}
		for _, key := range keys {
			if _, blocked := conflictSet[key]; blocked {
				continue
			}
			updateColumns = append(updateColumns, key)
		}
	}
	if len(updateColumns) == 0 {
		query += " DO NOTHING"
	} else {
		clauses := make([]string, 0, len(updateColumns))
		for _, col := range updateColumns {
			if !isIdentifier(col) {
				return nil, fmt.Errorf("invalid update column: %s", col)
			}
			q := quoteIdent(col)
			clauses = append(clauses, q+" = EXCLUDED."+q)
		}
		query += " DO UPDATE SET " + strings.Join(clauses, ", ")
	}

	returning, err := readColumnList(input["returning"], nil)
	if err != nil {
		return nil, err
	}
	if len(returning) > 0 {
		query += " RETURNING " + joinQuoted(returning)
		rows, qerr := db.QueryContext(ctx, query, args...)
		if qerr != nil {
			return nil, fmt.Errorf("postgres upsert failed: %w", qerr)
		}
		defer func() { _ = rows.Close() }()
		items, serr := scanRows(rows)
		if serr != nil {
			return nil, serr
		}
		return map[string]any{"rows_affected": len(items), "rows": items}, nil
	}

	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres upsert failed: %w", err)
	}
	affected, _ := res.RowsAffected()
	return map[string]any{"rows_affected": affected}, nil
}

func (c *Connector) queryTemplateAction(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	db, closeFn, err := openDB(ctx, auth)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	sqlText := strings.TrimSpace(readString(input, "sql", ""))
	if sqlText == "" {
		return nil, fmt.Errorf("sql is required")
	}
	params := readAnySlice(input["params"])
	rows, err := db.QueryContext(ctx, sqlText, params...)
	if err != nil {
		return nil, fmt.Errorf("postgres query template failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{"rows": items, "row_count": len(items)}, nil
}

func (c *Connector) execTemplateAction(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	db, closeFn, err := openDB(ctx, auth)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	sqlText := strings.TrimSpace(readString(input, "sql", ""))
	if sqlText == "" {
		return nil, fmt.Errorf("sql is required")
	}
	params := readAnySlice(input["params"])
	res, err := db.ExecContext(ctx, sqlText, params...)
	if err != nil {
		return nil, fmt.Errorf("postgres exec template failed: %w", err)
	}
	affected, _ := res.RowsAffected()
	return map[string]any{"rows_affected": affected}, nil
}

func openDB(ctx context.Context, auth map[string]string) (*sql.DB, func(), error) {
	dsn := strings.TrimSpace(auth["dsn"])
	if dsn == "" {
		host := strings.TrimSpace(auth["host"])
		dbName := strings.TrimSpace(auth["database"])
		user := strings.TrimSpace(auth["user"])
		password := strings.TrimSpace(auth["password"])
		if host == "" || dbName == "" || user == "" {
			return nil, nil, fmt.Errorf("either auth.dsn or auth.host/auth.database/auth.user is required")
		}
		port := strings.TrimSpace(auth["port"])
		if port == "" {
			port = "5432"
		}
		sslmode := strings.TrimSpace(auth["sslmode"])
		if sslmode == "" {
			sslmode = "prefer"
		}
		dsn = "host=" + host + " port=" + port + " dbname=" + dbName + " user=" + user + " password=" + password + " sslmode=" + sslmode
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open postgres connection: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, func() { _ = db.Close() }, nil
}

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}
	out := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range values {
			scanTargets[i] = &values[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		item := make(map[string]any, len(columns))
		for i, col := range columns {
			item[col] = normalizeSQLValue(values[i])
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return out, nil
}

func normalizeSQLValue(v any) any {
	switch typed := v.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}

func buildWhereSQL(raw any, startIndex int) (string, []any, error) {
	where, ok := raw.(map[string]any)
	if !ok || len(where) == 0 {
		return "", nil, nil
	}
	keys := sortedKeys(where)
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	next := startIndex
	for _, key := range keys {
		if !isIdentifier(key) {
			return "", nil, fmt.Errorf("invalid where field: %s", key)
		}
		clause, clauseArgs, err := buildWhereClause(key, where[key], next)
		if err != nil {
			return "", nil, err
		}
		clauses = append(clauses, clause)
		args = append(args, clauseArgs...)
		next += len(clauseArgs)
	}
	return strings.Join(clauses, " AND "), args, nil
}

func buildWhereClause(field string, raw any, idx int) (string, []any, error) {
	quoted := quoteIdent(field)
	if raw == nil {
		return quoted + " IS NULL", nil, nil
	}
	opExpr, ok := raw.(map[string]any)
	if !ok {
		return quoted + " = $" + strconv.Itoa(idx), []any{raw}, nil
	}
	op := strings.ToLower(strings.TrimSpace(readString(opExpr, "op", "eq")))
	value, hasValue := opExpr["value"]
	switch op {
	case "eq":
		if !hasValue || value == nil {
			return quoted + " IS NULL", nil, nil
		}
		return quoted + " = $" + strconv.Itoa(idx), []any{value}, nil
	case "ne":
		if !hasValue || value == nil {
			return quoted + " IS NOT NULL", nil, nil
		}
		return quoted + " <> $" + strconv.Itoa(idx), []any{value}, nil
	case "gt", "gte", "lt", "lte", "like", "ilike":
		if !hasValue {
			return "", nil, fmt.Errorf("where.%s requires value for op %s", field, op)
		}
		symbol := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<=", "like": "LIKE", "ilike": "ILIKE"}[op]
		return quoted + " " + symbol + " $" + strconv.Itoa(idx), []any{value}, nil
	case "in":
		vals := readAnySlice(value)
		if len(vals) == 0 {
			return "", nil, fmt.Errorf("where.%s in requires non-empty value array", field)
		}
		parts := make([]string, 0, len(vals))
		for i := range vals {
			parts = append(parts, "$"+strconv.Itoa(idx+i))
		}
		return quoted + " IN (" + strings.Join(parts, ", ") + ")", vals, nil
	default:
		return "", nil, fmt.Errorf("unsupported where op %q on %s", op, field)
	}
}

func buildOrderBy(raw any) (string, error) {
	items := readAnySlice(raw)
	if len(items) == 0 {
		return "", nil
	}
	clauses := make([]string, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return "", fmt.Errorf("order_by items must be objects")
		}
		col := strings.TrimSpace(readString(obj, "column", ""))
		if !isIdentifier(col) {
			return "", fmt.Errorf("invalid order_by column: %s", col)
		}
		desc := readBool(obj, "desc", false)
		dir := "ASC"
		if desc {
			dir = "DESC"
		}
		clauses = append(clauses, quoteIdent(col)+" "+dir)
	}
	return strings.Join(clauses, ", "), nil
}

func readRequiredIdentifier(input map[string]any, key string) (string, error) {
	value := strings.TrimSpace(readString(input, key, ""))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	if !isIdentifier(value) {
		return "", fmt.Errorf("invalid %s: %s", key, value)
	}
	return value, nil
}

func readColumnList(raw any, fallback []string) ([]string, error) {
	items := readAnySlice(raw)
	if len(items) == 0 {
		if fallback == nil {
			return nil, nil
		}
		return append([]string(nil), fallback...), nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text == "" {
			continue
		}
		if text == "*" {
			out = append(out, text)
			continue
		}
		if !isIdentifier(text) {
			return nil, fmt.Errorf("invalid column name: %s", text)
		}
		out = append(out, text)
	}
	if len(out) == 0 && fallback != nil {
		return append([]string(nil), fallback...), nil
	}
	return out, nil
}

func joinQuoted(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, col := range columns {
		quoted = append(quoted, quoteIdent(col))
	}
	return strings.Join(quoted, ", ")
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isIdentifier(name string) bool {
	return identRE.MatchString(name)
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func readObject(input map[string]any, key string) (map[string]any, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return nil, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return obj, nil
}

func readString(input map[string]any, key, fallback string) string {
	raw, ok := input[key]
	if !ok || raw == nil {
		return fallback
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return fallback
}

func readBool(input map[string]any, key string, fallback bool) bool {
	raw, ok := input[key]
	if !ok || raw == nil {
		return fallback
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func readInt(input map[string]any, key string, fallback int) int {
	raw, ok := input[key]
	if !ok || raw == nil {
		return fallback
	}
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func readAnySlice(raw any) []any {
	if raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func schemaRows() map[string]any {
	return map[string]any{"type": "object"}
}

func schemaMutation() map[string]any {
	return map[string]any{"type": "object"}
}

func schemaSelect() map[string]any {
	return map[string]any{"type": "object", "required": []string{"table"}}
}

func schemaInsert() map[string]any {
	return map[string]any{"type": "object", "required": []string{"table", "values"}}
}

func schemaUpdate() map[string]any {
	return map[string]any{"type": "object", "required": []string{"table", "set"}}
}

func schemaDelete() map[string]any {
	return map[string]any{"type": "object", "required": []string{"table"}}
}

func schemaUpsert() map[string]any {
	return map[string]any{"type": "object", "required": []string{"table", "values", "conflict_columns"}}
}

func schemaQueryTemplate() map[string]any {
	return map[string]any{"type": "object", "required": []string{"sql"}}
}

var _ connectors.Connector = (*Connector)(nil)
