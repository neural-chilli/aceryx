package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/llm"
	"github.com/neural-chilli/aceryx/internal/mcp"
	"github.com/neural-chilli/aceryx/internal/rag"
)

type ToolManifest struct {
	tools       []ResolvedTool
	toolsByName map[string]*ResolvedTool
}

func NewToolManifest(tools []ResolvedTool) *ToolManifest {
	m := &ToolManifest{
		tools:       make([]ResolvedTool, 0, len(tools)),
		toolsByName: make(map[string]*ResolvedTool, len(tools)),
	}
	for _, t := range tools {
		tc := t
		m.tools = append(m.tools, tc)
		m.toolsByName[tc.Name] = &m.tools[len(m.tools)-1]
	}
	return m
}

func (m *ToolManifest) Tools() []ResolvedTool {
	if m == nil {
		return nil
	}
	out := make([]ResolvedTool, len(m.tools))
	copy(out, m.tools)
	return out
}

func (m *ToolManifest) ToLLMToolDefs() []llm.ToolDef {
	if m == nil {
		return nil
	}
	out := make([]llm.ToolDef, 0, len(m.tools))
	for _, tool := range m.tools {
		params := map[string]any{}
		if len(tool.Parameters) > 0 {
			_ = json.Unmarshal(tool.Parameters, &params)
		}
		if len(params) == 0 {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, llm.ToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
		})
	}
	return out
}

type ResolvedTool struct {
	ID          string
	Name        string
	Description string
	Parameters  json.RawMessage
	Source      ToolSource
	ToolSafety  string
	Invoker     ToolInvoker
}

type ToolSource string

const (
	ToolSourceConnector ToolSource = "connector"
	ToolSourceMCP       ToolSource = "mcp"
	ToolSourceRAG       ToolSource = "rag"
	ToolSourceCaseData  ToolSource = "case_data"
)

type ToolInvoker interface {
	Invoke(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error)
}

type MCPManager interface {
	DiscoverTools(ctx context.Context, tenantID uuid.UUID, serverURL string, auth mcp.AuthConfig) ([]mcp.MCPTool, error)
}

type ToolAssembler struct {
	mcpManager MCPManager
	ragSearch  *rag.SearchService
}

func NewToolAssembler(mcpManager MCPManager, ragSearch *rag.SearchService) *ToolAssembler {
	return &ToolAssembler{mcpManager: mcpManager, ragSearch: ragSearch}
}

func (ta *ToolAssembler) Assemble(ctx context.Context, tenantID uuid.UUID, policy ToolPolicy, toolNodes []ToolNodeConfig, invokerFactory func(node ToolNodeConfig, toolName string) (ToolInvoker, string, json.RawMessage, error)) (*ToolManifest, error) {
	allowedRefs := make(map[string]struct{}, len(policy.Tools))
	for _, ref := range policy.Tools {
		if strings.TrimSpace(ref.Ref) == "" {
			continue
		}
		allowedRefs[strings.TrimSpace(ref.Ref)] = struct{}{}
	}
	if len(allowedRefs) == 0 {
		return NewToolManifest(nil), nil
	}

	mode := policy.ToolMode.Normalize()
	tools := make([]ResolvedTool, 0, len(toolNodes))
	for _, node := range toolNodes {
		if _, ok := allowedRefs[strings.TrimSpace(node.ID)]; !ok {
			continue
		}
		name := sanitizeToolName(node)
		invoker, safety, params, err := invokerFactory(node, name)
		if err != nil {
			return nil, fmt.Errorf("build tool %s: %w", node.ID, err)
		}
		if !allowedByMode(mode, safety) {
			continue
		}
		source := ToolSource(strings.TrimSpace(node.Source))
		if source == "" {
			source = ToolSourceConnector
		}
		tools = append(tools, ResolvedTool{
			ID:          node.ID,
			Name:        name,
			Description: strings.TrimSpace(node.Description),
			Parameters:  params,
			Source:      source,
			ToolSafety:  safety,
			Invoker:     invoker,
		})
	}
	return NewToolManifest(tools), nil
}

func sanitizeToolName(node ToolNodeConfig) string {
	if strings.TrimSpace(node.MCPPrefix) != "" && strings.TrimSpace(node.MCPToolName) != "" {
		return normalizeToolName(node.MCPPrefix + "_" + node.MCPToolName)
	}
	base := node.Connector
	if strings.TrimSpace(base) == "" {
		base = node.ID
	}
	return normalizeToolName(base)
}

func normalizeToolName(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "tool"
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_':
			b.WriteRune(r)
		case r == '-' || r == ' ' || r == '.':
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "tool"
	}
	return out
}

func allowedByMode(mode ToolMode, safety string) bool {
	switch mode {
	case ToolModeReadOnly:
		return safety == "read_only" || safety == ""
	case ToolModeRestrictedWrite:
		return safety == "read_only" || safety == "" || safety == "idempotent_write"
	case ToolModeFull:
		return true
	default:
		return safety == "read_only" || safety == ""
	}
}
