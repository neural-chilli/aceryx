package reports

import (
	"encoding/json"
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

type SQLInspector struct {
	Views     []string
	Functions map[string]struct{}
}

func NewSQLInspector() SQLInspector {
	return SQLInspector{
		Views:     append([]string{}, ApprovedViews...),
		Functions: AllowedFunctions,
	}
}

func (i SQLInspector) Validate(sqlText string) error {
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return fmt.Errorf("sql is required")
	}
	tree, err := pg_query.Parse(sqlText)
	if err != nil {
		return fmt.Errorf("unparseable SQL")
	}
	if len(tree.Stmts) != 1 {
		return fmt.Errorf("exactly one SQL statement is required")
	}
	raw := tree.Stmts[0]
	if raw.Stmt == nil {
		return fmt.Errorf("invalid SQL statement")
	}
	if _, ok := raw.Stmt.Node.(*pg_query.Node_SelectStmt); !ok {
		return fmt.Errorf("only SELECT statements are allowed")
	}

	jsonTree, err := pg_query.ParseToJSON(sqlText)
	if err != nil {
		return fmt.Errorf("unparseable SQL")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonTree), &parsed); err != nil {
		return fmt.Errorf("decode parse tree: %w", err)
	}
	return i.validateNode(parsed)
}

func (i SQLInspector) ScopeToTenant(sqlText string) (string, error) {
	if err := i.Validate(sqlText); err != nil {
		return "", err
	}
	cte := make([]string, 0, len(i.Views)+1)
	for _, view := range i.Views {
		view = strings.ToLower(strings.TrimSpace(view))
		cte = append(cte, fmt.Sprintf("%s AS (SELECT * FROM public.%s WHERE tenant_id = $1)", view, view))
	}
	cte = append(cte, fmt.Sprintf("__acx_report AS (%s)", sqlText))
	return fmt.Sprintf("WITH %s SELECT * FROM __acx_report LIMIT $2", strings.Join(cte, ", ")), nil
}

func (i SQLInspector) validateNode(node any) error {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if isBlockedStatementNode(key) {
				return fmt.Errorf("disallowed SQL statement type")
			}
			if key == "RangeVar" {
				mv, ok := child.(map[string]any)
				if ok {
					rel, _ := mv["relname"].(string)
					rel = strings.ToLower(strings.TrimSpace(rel))
					schema, _ := mv["schemaname"].(string)
					schema = strings.ToLower(strings.TrimSpace(schema))
					if schema != "" {
						return fmt.Errorf("schema-qualified references are not allowed")
					}
					if strings.HasPrefix(rel, "pg_") || strings.HasPrefix(schema, "pg_") {
						return fmt.Errorf("system catalog access is not allowed")
					}
					if !containsString(i.Views, rel) {
						return fmt.Errorf("unapproved table/view reference: %s", rel)
					}
				}
			}
			if key == "FuncCall" {
				mv, ok := child.(map[string]any)
				if ok {
					name := extractFuncName(mv)
					if name != "" {
						if _, ok := i.Functions[name]; !ok {
							return fmt.Errorf("unapproved function: %s", name)
						}
					}
				}
			}
			if err := i.validateNode(child); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := i.validateNode(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractFuncName(call map[string]any) string {
	raw, ok := call["funcname"].([]any)
	if !ok || len(raw) == 0 {
		return ""
	}
	last := raw[len(raw)-1]
	node, ok := last.(map[string]any)
	if !ok {
		return ""
	}
	stringNode, ok := node["String"].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := stringNode["sval"].(string)
	return strings.ToLower(strings.TrimSpace(name))
}

func isBlockedStatementNode(nodeName string) bool {
	switch nodeName {
	case "InsertStmt", "UpdateStmt", "DeleteStmt", "MergeStmt", "CreateStmt", "CreateTableAsStmt", "CreateFunctionStmt", "CreateRoleStmt", "AlterTableStmt", "DropStmt", "GrantStmt", "GrantRoleStmt", "RevokeStmt", "TruncateStmt", "IndexStmt", "VacuumStmt":
		return true
	default:
		return false
	}
}

func containsString(values []string, value string) bool {
	for _, v := range values {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}
