package mcpserver

import (
	"sort"
	"strings"

	"github.com/google/uuid"
)

func (h *Handler) visibleTools(conn *Connection) []ToolDefinition {
	if h == nil {
		return nil
	}
	visible := make([]ToolDefinition, 0, len(h.tools))
	for _, tool := range h.tools {
		if h.isToolDisabled(tool.Name()) {
			continue
		}
		if !hasPermission(conn, tool.RequiredPermission()) {
			continue
		}
		if !h.isTenantFeatureEnabled(conn, tool.Name()) {
			continue
		}
		visible = append(visible, tool.Definition())
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].Name < visible[j].Name })
	return visible
}

func (h *Handler) isToolDisabled(name string) bool {
	cfg := h.config.WithDefaults()
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	disabled := toSet(cfg.DisabledTools)
	if disabled[name] {
		return true
	}
	if len(cfg.EnabledTools) == 0 {
		return false
	}
	enabled := toSet(cfg.EnabledTools)
	return !enabled[name]
}

func (h *Handler) isTenantFeatureEnabled(conn *Connection, toolName string) bool {
	if strings.TrimSpace(toolName) != "search_knowledge_base" {
		return true
	}
	if h.tenantFeatures == nil {
		return true
	}
	return h.tenantFeatures.IsRAGEnabled(conn.TenantID)
}

func toSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out[trimmed] = true
	}
	return out
}

func hasPermission(conn *Connection, permission string) bool {
	if conn == nil {
		return false
	}
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return true
	}
	for _, role := range conn.Roles {
		role = strings.TrimSpace(role)
		if role == "*" || role == permission {
			return true
		}
		if idx := strings.IndexByte(permission, ':'); idx > 0 {
			prefix := permission[:idx]
			if role == prefix+":*" {
				return true
			}
		}
	}
	return false
}

type TenantFeatureChecker interface {
	IsRAGEnabled(tenantID uuid.UUID) bool
}
