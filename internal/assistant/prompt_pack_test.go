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
		&PromptPackInput{ContractVersion: BuilderContractVersion, FrontendContext: "frontend_ctx_block"},
	)

	if !strings.Contains(out, "backend_prompt_pack:") {
		t.Fatalf("expected backend prompt pack in composed prompt")
	}
	if !strings.Contains(out, "frontend_prompt_pack:") {
		t.Fatalf("expected frontend prompt pack in composed prompt")
	}
	if !strings.Contains(out, "assistant_contract_version: "+BuilderContractVersion) {
		t.Fatalf("expected assistant contract version boundary in composed prompt")
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

func TestAssertBuilderContractVersion(t *testing.T) {
	t.Run("accepts builder contract version for describe mode", func(t *testing.T) {
		err := assertBuilderContractVersion(ModeDescribe, "builder", &PromptPackInput{ContractVersion: BuilderContractVersion})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("rejects missing builder contract version in describe mode", func(t *testing.T) {
		err := assertBuilderContractVersion(ModeDescribe, "builder", &PromptPackInput{})
		if err == nil {
			t.Fatal("expected missing contract version error")
		}
	})

	t.Run("skips assertion for explain mode", func(t *testing.T) {
		err := assertBuilderContractVersion(ModeExplain, "builder", nil)
		if err != nil {
			t.Fatalf("expected no error for explain mode, got %v", err)
		}
	})
}
