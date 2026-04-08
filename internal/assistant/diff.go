package assistant

import "strings"

func normalizeMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case ModeDescribe, ModeRefactor, ModeExplain, ModeTestGenerate:
		return strings.TrimSpace(strings.ToLower(mode))
	default:
		return ModeDescribe
	}
}

func computeUnifiedDiff(before, after string) string {
	if before == after {
		return ""
	}
	beforeLines := splitLines(before)
	afterLines := splitLines(after)

	var out []string
	out = append(out, "--- before")
	out = append(out, "+++ after")
	out = append(out, "@@ -1 +1 @@")

	for _, line := range beforeLines {
		out = append(out, "-"+line)
	}
	for _, line := range afterLines {
		out = append(out, "+"+line)
	}
	return strings.Join(out, "\n")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func extractYAML(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "```") {
		return trimmed
	}
	start := strings.Index(trimmed, "```")
	if start < 0 {
		return trimmed
	}
	rest := trimmed[start+3:]
	nl := strings.Index(rest, "\n")
	if nl < 0 {
		return trimmed
	}
	lang := strings.TrimSpace(rest[:nl])
	body := rest[nl+1:]
	end := strings.Index(body, "```")
	if end < 0 {
		return trimmed
	}
	if lang == "" || strings.EqualFold(lang, "yaml") || strings.EqualFold(lang, "yml") {
		return strings.TrimSpace(body[:end])
	}
	return strings.TrimSpace(body[:end])
}
