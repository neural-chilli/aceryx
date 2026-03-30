package reports

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	jsonTree, err := pg_query.ParseToJSON(sqlText)
	if err != nil {
		return "", fmt.Errorf("parse SQL for scoping: %w", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonTree), &parsed); err != nil {
		return "", fmt.Errorf("decode parse tree for scoping: %w", err)
	}
	refs := collectRangeVars(parsed)
	rewritten := sqlText
	for _, ref := range refs {
		if !containsString(i.Views, ref.RelName) {
			continue
		}
		alias := strings.TrimSpace(ref.Alias)
		if alias == "" {
			alias = ref.RelName
		}
		pattern := regexp.MustCompile(`(?is)\b(from|join)\s+((?:[a-zA-Z_][a-zA-Z0-9_]*\.)?` + regexp.QuoteMeta(ref.RelName) + `)\b(?:\s+(?:as\s+)?([a-zA-Z_][a-zA-Z0-9_]*))?`)
		// Use $$1 so regexp replacement emits literal SQL placeholder "$1" (not capture group 1).
		rewritten = replaceFirstRegex(rewritten, pattern, fmt.Sprintf(`${1} (SELECT * FROM %s WHERE tenant_id = $$1) AS %s`, ref.RelName, alias))
	}
	return fmt.Sprintf("SELECT * FROM (%s) AS __acx_report LIMIT $2", rewritten), nil
}

type rangeRef struct {
	RelName string
	Alias   string
}

func collectRangeVars(node any) []rangeRef {
	refs := make([]rangeRef, 0)
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "RangeVar" {
				mv, ok := child.(map[string]any)
				if !ok {
					continue
				}
				rel, _ := mv["relname"].(string)
				if strings.TrimSpace(rel) == "" {
					continue
				}
				alias := ""
				if aliasRaw, ok := mv["alias"].(map[string]any); ok {
					if aliasName, ok := aliasRaw["aliasname"].(string); ok {
						alias = aliasName
					}
				}
				refs = append(refs, rangeRef{RelName: strings.ToLower(rel), Alias: strings.ToLower(alias)})
			}
			refs = append(refs, collectRangeVars(child)...)
		}
	case []any:
		for _, item := range v {
			refs = append(refs, collectRangeVars(item)...)
		}
	}
	return dedupeRangeRefs(refs)
}

func dedupeRangeRefs(in []rangeRef) []rangeRef {
	seen := map[string]struct{}{}
	out := make([]rangeRef, 0, len(in))
	for _, ref := range in {
		k := ref.RelName + "|" + ref.Alias
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, ref)
	}
	return out
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

func replaceFirstRegex(input string, pattern *regexp.Regexp, replacement string) string {
	loc := pattern.FindStringIndex(input)
	if loc == nil {
		return input
	}
	match := input[loc[0]:loc[1]]
	replaced := pattern.ReplaceAllString(match, replacement)
	return input[:loc[0]] + replaced + input[loc[1]:]
}
