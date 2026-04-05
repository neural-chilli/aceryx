package mcp

import (
	"fmt"
	"regexp"
	"strings"
)

var nonWordRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func PrefixToolName(serverPrefix, toolName string) string {
	return fmt.Sprintf("mcp_%s_%s", sanitize(serverPrefix), sanitize(toolName))
}

func UnprefixToolName(prefixedName string) (serverPrefix, toolName string, err error) {
	if !strings.HasPrefix(prefixedName, "mcp_") {
		return "", "", fmt.Errorf("invalid prefixed tool name")
	}
	remainder := strings.TrimPrefix(prefixedName, "mcp_")
	parts := strings.SplitN(remainder, "_", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid prefixed tool name")
	}
	return parts[0], parts[1], nil
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = nonWordRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	s = strings.ToLower(s)
	if s == "" {
		return "server"
	}
	return s
}
