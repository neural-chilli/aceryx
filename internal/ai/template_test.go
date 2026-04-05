package ai

import (
	"strings"
	"testing"
)

func TestRenderPromptResolvesInputAndConfig(t *testing.T) {
	out, err := RenderPrompt("Text={{.Input.text}} Tone={{.Config.tone}} Case={{.Case.ID}}", PromptData{
		Input:  map[string]string{"text": "hello"},
		Config: map[string]string{"tone": "friendly"},
		Case:   CaseContext{ID: "c-1", Type: "support"},
	})
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}
	if out != "Text=hello Tone=friendly Case=c-1" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRenderPromptMissingInputDefaultsEmpty(t *testing.T) {
	out, err := RenderPrompt("X={{.Input.missing}}", PromptData{})
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}
	if out != "X=" {
		t.Fatalf("expected missing input to render empty, got %q", out)
	}
}

func TestRenderPromptMalformedTemplate(t *testing.T) {
	_, err := RenderPrompt("{{.Input.text", PromptData{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "{{.Input.text") {
		t.Fatalf("expected template content in error, got %v", err)
	}
}

func TestRenderPromptForbiddenDirective(t *testing.T) {
	_, err := RenderPrompt("{{template \"x\" .}}", PromptData{})
	if err == nil {
		t.Fatalf("expected forbidden directive error")
	}
}
