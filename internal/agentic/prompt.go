package agentic

import (
	"encoding/json"
	"fmt"
	"strings"
)

func buildSystemPrompt(manifest *ToolManifest, outputSchema json.RawMessage) string {
	toolLines := make([]string, 0)
	if manifest != nil {
		for _, tool := range manifest.tools {
			toolLines = append(toolLines, fmt.Sprintf("- %s (%s): %s", tool.Name, tool.Source, tool.Description))
		}
	}
	if len(toolLines) == 0 {
		toolLines = append(toolLines, "- No tools available.")
	}
	return strings.TrimSpace(fmt.Sprintf(`
You are an Aceryx agentic reasoning assistant.
Reason step by step about the goal.
Use available tools when needed before concluding.
Return a final JSON conclusion that matches this schema exactly:
%s

Rules:
1. Use only declared tools.
2. Provide a confidence value from 0.0 to 1.0 in the conclusion.
3. Include a "reasoning" array of key factors in the final JSON.
4. If constraints are nearing limits, conclude with best available evidence.

Available tools:
%s
`, string(outputSchema), strings.Join(toolLines, "\n")))
}

func buildGoalPrompt(goal string, caseData json.RawMessage, outputSchema json.RawMessage) string {
	return strings.TrimSpace(fmt.Sprintf(`
Goal:
%s

Case data snapshot:
%s

Output schema:
%s
`, strings.TrimSpace(goal), string(caseData), string(outputSchema)))
}
