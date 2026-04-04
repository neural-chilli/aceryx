package reports

import (
	"regexp"
	"strings"
)

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
