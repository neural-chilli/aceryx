package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"
)

func renderPromptTemplate(raw string, data map[string]any) (string, error) {
	funcs := template.FuncMap{
		"toJSON": func(v any) string {
			buf, err := json.Marshal(v)
			if err != nil {
				return "{}"
			}
			return string(buf)
		},
		"formatCurrency": func(v any) string {
			n, ok := asFloat(v)
			if !ok {
				return ""
			}
			return formatCurrency(n)
		},
		"formatDate": func(v any) string {
			t, ok := asTime(v)
			if !ok {
				return ""
			}
			return t.Format("2 January 2006")
		},
	}
	tpl, err := template.New("prompt").Funcs(funcs).Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse prompt template: %w", err)
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render prompt template: %w", err)
	}
	return out.String(), nil
}

func formatCurrency(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	frac := "00"
	if len(parts) == 2 {
		frac = parts[1]
	}
	neg := false
	if strings.HasPrefix(intPart, "-") {
		neg = true
		intPart = strings.TrimPrefix(intPart, "-")
	}
	for i := len(intPart) - 3; i > 0; i -= 3 {
		intPart = intPart[:i] + "," + intPart[i:]
	}
	if neg {
		intPart = "-" + intPart
	}
	return "£" + intPart + "." + frac
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	default:
		return time.Time{}, false
	}
}
