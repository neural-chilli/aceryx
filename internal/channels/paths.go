package channels

import "strings"

func lookupPath(in map[string]any, path string) (any, bool) {
	if len(in) == 0 || strings.TrimSpace(path) == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var current any = in
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[part]
		if !ok {
			return nil, false
		}
		current = v
	}
	return current, true
}

func setPath(in map[string]any, path string, value any) {
	if strings.TrimSpace(path) == "" {
		return
	}
	parts := strings.Split(path, ".")
	current := in
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}
