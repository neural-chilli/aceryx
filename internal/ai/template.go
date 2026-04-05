package ai

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"
)

type PromptData struct {
	Input  map[string]string `json:"input"`
	Config map[string]string `json:"config"`
	Case   CaseContext       `json:"case"`
}

type CaseContext struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

var forbiddenTemplateDirective = regexp.MustCompile(`\{\{\s*(template|block|define|call)\b`)

func RenderPrompt(templateStr string, data PromptData) (string, error) {
	raw := strings.TrimSpace(templateStr)
	if raw == "" {
		return "", fmt.Errorf("template is empty")
	}
	if forbiddenTemplateDirective.MatchString(strings.ToLower(raw)) {
		return "", fmt.Errorf("template contains forbidden directive (template/block/define/call): %q", templateStr)
	}
	tpl, err := template.New("ai_component_prompt").Option("missingkey=zero").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse prompt template %q: %w", templateStr, err)
	}
	if data.Input == nil {
		data.Input = map[string]string{}
	}
	if data.Config == nil {
		data.Config = map[string]string{}
	}

	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		var out bytes.Buffer
		execErr := tpl.Execute(&out, data)
		ch <- result{text: out.String(), err: execErr}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return "", fmt.Errorf("execute prompt template %q: %w", templateStr, res.err)
		}
		return res.text, nil
	case <-time.After(time.Second):
		return "", fmt.Errorf("execute prompt template %q: timed out after 1s", templateStr)
	}
}
