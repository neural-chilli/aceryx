package assistant

import (
	"fmt"
	"strings"
)

const BuilderContractVersion = "2026-04-08-a1"

func assistantSystemPrompt() string {
	return strings.TrimSpace(`
You are the Aceryx Workflow Builder assistant.

Aceryx is a case orchestration platform for business workflows with human tasks, AI steps, integrations, rules, timers, notifications, and audit-safe execution.
Your job is to produce executable workflow YAML for the Builder, not prose.

For mode describe/refactor:
- Output ONLY canonical Builder AST YAML.
- Never output markdown fences.
- Never output commentary.

Canonical AST contract:
steps:
  - id: "step_id"
    type: "human_task|agent|ai_component|extraction|integration|rule|timer|notification"
    depends_on: []
    config: {}
    position: { x: 0, y: 0 }

Hard correctness rules:
- Always include top-level "steps" array.
- Never wrap in "workflow:".
- Use only step types and capabilities provided in context.
- Integration steps must use config.connector + config.action.
- Human forms must use config.form_schema.fields[].bind (never key).
- Prefer concrete mappings and executable defaults over placeholders.
- If an explicitly requested capability is unavailable, use a clearly named placeholder step.

Data path conventions:
- Case data is read/written under case.data.*
- Step outputs are available at case.steps.<step_id>.result
- Keep output paths explicit and stable.

If mode is explain: explain the workflow in concise plain English.
If mode is test_generate: output concise Gherkin scenarios.
`)
}

func backendBuilderPromptPack() string {
	return strings.TrimSpace(`
Aceryx Builder backend prompt pack:

Step-level required config guidance:
- human_task:
  - include assign_to_role or assign_to_user
  - include config.form_schema with fields and actions
- agent:
  - include prompt_template
  - include context (array)
  - include output_schema (object)
  - include on_low_confidence: escalate_to_human|proceed
- ai_component:
  - include component
  - include input_paths (object)
  - include output_path
- extraction:
  - include document_path, schema, output_path
  - include on_review and on_reject when review path is expected
- integration:
  - include connector and action
  - include config.input object for request parameters
  - include output_mapping when persisting integration outputs
- rule:
  - include outcomes map and meaningful conditions
- timer:
  - include duration (e.g. "5m", "24h")
- notification:
  - include channel and message/template fields

Form schema guidance:
- Use config.form_schema.title
- fields[] should include bind, label, type, required
- bind should map to usable runtime fields (often decision.* or explicit result keys)
- actions[] should include value labels such as approve/reject/submit

Mapping guidance:
- For integrations, map input values from case.data.* or case.steps.<id>.result.*
- For DB writes, include table and values payload in config.input
- For extraction and AI steps, include deterministic output_path under case.data.*
`)
}

func composeAssistantUserPrompt(mode, userPrompt, yamlBefore, pageContext string, promptPack *PromptPackInput) string {
	var b strings.Builder
	b.WriteString("mode: ")
	b.WriteString(mode)
	b.WriteString("\n\n")

	pageContext = strings.TrimSpace(pageContext)
	if pageContext == "" {
		pageContext = "builder"
	}
	b.WriteString("page_context: ")
	b.WriteString(pageContext)
	b.WriteString("\n\n")

	if yamlBefore != "" {
		b.WriteString("current_workflow_yaml:\n")
		b.WriteString(strings.TrimSpace(yamlBefore))
		b.WriteString("\n\n")
	}

	b.WriteString("user_request:\n")
	b.WriteString(strings.TrimSpace(userPrompt))
	b.WriteString("\n\n")

	if pageContext == "builder" {
		b.WriteString("backend_prompt_pack:\n")
		b.WriteString(backendBuilderPromptPack())
		b.WriteString("\n\n")
		if promptPack != nil && strings.TrimSpace(promptPack.ContractVersion) != "" {
			b.WriteString("assistant_contract_version: ")
			b.WriteString(strings.TrimSpace(promptPack.ContractVersion))
			b.WriteString("\n\n")
		}
		if promptPack != nil && strings.TrimSpace(promptPack.FrontendContext) != "" {
			b.WriteString("frontend_prompt_pack:\n")
			b.WriteString(strings.TrimSpace(promptPack.FrontendContext))
		}
	}

	return strings.TrimSpace(b.String())
}

func assertBuilderContractVersion(mode, pageContext string, promptPack *PromptPackInput) error {
	mode = normalizeMode(mode)
	if mode != ModeDescribe && mode != ModeRefactor {
		return nil
	}
	if strings.TrimSpace(pageContext) != "builder" {
		return nil
	}
	if promptPack == nil {
		return fmt.Errorf("prompt_pack.contract_version is required")
	}
	version := strings.TrimSpace(promptPack.ContractVersion)
	if version == "" {
		return fmt.Errorf("prompt_pack.contract_version is required")
	}
	if version != BuilderContractVersion {
		return fmt.Errorf("prompt_pack.contract_version %q is not supported", version)
	}
	return nil
}
