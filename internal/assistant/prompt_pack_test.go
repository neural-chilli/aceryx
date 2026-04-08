package assistant

import (
	"strings"
	"testing"
)

func TestComposeAssistantUserPrompt_BuilderIncludesBothPacks(t *testing.T) {
	out := composeAssistantUserPrompt(
		ModeDescribe,
		"Build onboarding flow",
		"steps: []",
		"builder",
		&PromptPackInput{FrontendContext: "frontend_ctx_block"},
	)

	if !strings.Contains(out, "backend_prompt_pack:") {
		t.Fatalf("expected backend prompt pack in composed prompt")
	}
	if !strings.Contains(out, "frontend_prompt_pack:") {
		t.Fatalf("expected frontend prompt pack in composed prompt")
	}
	if !strings.Contains(out, "frontend_ctx_block") {
		t.Fatalf("expected frontend context content in composed prompt")
	}
	if !strings.Contains(out, "current_workflow_yaml:") {
		t.Fatalf("expected current workflow yaml section")
	}
}

func TestComposeAssistantUserPrompt_NonBuilderSkipsPacks(t *testing.T) {
	out := composeAssistantUserPrompt(
		ModeExplain,
		"Explain",
		"",
		"reports",
		&PromptPackInput{FrontendContext: "frontend_ctx_block"},
	)
	if strings.Contains(out, "backend_prompt_pack:") {
		t.Fatalf("did not expect backend prompt pack for non-builder context")
	}
	if strings.Contains(out, "frontend_prompt_pack:") {
		t.Fatalf("did not expect frontend prompt pack for non-builder context")
	}
}
