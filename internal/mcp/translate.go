package mcp

import (
	"encoding/json"

	"github.com/neural-chilli/aceryx/internal/llm"
)

func ToLLMToolDefs(tools []MCPTool, prefix string, filter []string) []llm.ToolDef {
	allowed := map[string]struct{}{}
	for _, name := range filter {
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}
	out := make([]llm.ToolDef, 0, len(tools))
	for _, tool := range tools {
		if len(allowed) > 0 {
			if _, ok := allowed[tool.Name]; !ok {
				continue
			}
		}
		params := map[string]any{}
		if len(tool.InputSchema) > 0 {
			_ = jsonUnmarshal(tool.InputSchema, &params)
		}
		out = append(out, llm.ToolDef{
			Name:        PrefixToolName(prefix, tool.Name),
			Description: tool.Description,
			Parameters:  params,
		})
	}
	return out
}

func jsonUnmarshal(raw []byte, out *map[string]any) error {
	return json.Unmarshal(raw, out)
}
