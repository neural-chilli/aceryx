package connectors

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var templatePattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

func ResolveTemplateString(raw string, ctx map[string]any) string {
	if raw == "" {
		return ""
	}
	return templatePattern.ReplaceAllStringFunc(raw, func(token string) string {
		matches := templatePattern.FindStringSubmatch(token)
		if len(matches) < 2 {
			return ""
		}
		path := strings.TrimSpace(matches[1])
		if path == "now" {
			return time.Now().UTC().Format(time.RFC3339)
		}
		if strings.HasPrefix(path, "secrets.") {
			key := strings.TrimPrefix(path, "secrets.")
			if resolver, ok := ctx["__secret_resolver"].(func(string) string); ok {
				return resolver(key)
			}
			return ""
		}
		value, ok := lookupDotPath(ctx, path)
		if !ok || value == nil {
			return ""
		}
		return stringifyTemplateValue(value)
	})
}

func ResolveTemplateAny(value any, ctx map[string]any) any {
	switch v := value.(type) {
	case string:
		return ResolveTemplateString(v, ctx)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			out[k] = ResolveTemplateAny(child, ctx)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = ResolveTemplateAny(child, ctx)
		}
		return out
	default:
		return value
	}
}

func lookupDotPath(root map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = root
	for _, part := range parts {
		switch typed := cur.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			cur = next
		case map[string]string:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			cur = next
		default:
			return nil, false
		}
	}
	return cur, true
}

func stringifyTemplateValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
