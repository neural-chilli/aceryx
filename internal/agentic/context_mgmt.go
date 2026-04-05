package agentic

import (
	"fmt"
	"strings"

	"github.com/neural-chilli/aceryx/internal/llm"
)

func summariseOldToolResults(messages []llm.Message, maxMessages int) []llm.Message {
	if maxMessages <= 0 || len(messages) <= maxMessages {
		return messages
	}
	keepTail := maxMessages - 2
	if keepTail < 1 {
		keepTail = 1
	}
	head := messages[0]
	tail := messages[len(messages)-keepTail:]
	summaryParts := make([]string, 0)
	for _, msg := range messages[1 : len(messages)-keepTail] {
		if msg.Role != "tool" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if len(content) > 120 {
			content = content[:120] + "..."
		}
		summaryParts = append(summaryParts, content)
	}
	summary := "No earlier tool outputs."
	if len(summaryParts) > 0 {
		summary = fmt.Sprintf("Previously observed tool outputs: %s", strings.Join(summaryParts, " | "))
	}
	out := []llm.Message{
		head,
		{Role: "user", Content: summary},
	}
	out = append(out, tail...)
	return out
}
