package assistant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNormalizeToBuilderASTYAML_CanonicalizesLegacyFields(t *testing.T) {
	input := `steps:
  - id: create_customer_onboarding_case
    type: human_task
    depends_on: []
    config:
      name: Create customer onboarding case
      form:
        fields:
          - key: customer_details_pdf
            label: Customer Details PDF
            type: file
            required: true
  - id: extract_customer_details
    type: integration
    depends_on: [create_customer_onboarding_case]
    config:
      integration: document_ingestion
      action: extract
`

	normalized, err := normalizeToBuilderASTYAML(input)
	if err != nil {
		t.Fatalf("normalizeToBuilderASTYAML returned error: %v", err)
	}

	var ast map[string]any
	if err := yaml.Unmarshal([]byte(normalized), &ast); err != nil {
		t.Fatalf("unmarshal normalized yaml: %v", err)
	}

	steps, ok := ast["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("expected 2 steps in normalized yaml")
	}

	firstStep := steps[0].(map[string]any)
	firstConfig := firstStep["config"].(map[string]any)
	formSchema, ok := firstConfig["form_schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected form_schema on human task config")
	}
	fields, ok := formSchema["fields"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("expected one form field in form_schema")
	}
	field := fields[0].(map[string]any)
	if got := field["bind"]; got != "customer_details_pdf" {
		t.Fatalf("expected bind to be derived from key, got: %#v", got)
	}

	secondStep := steps[1].(map[string]any)
	secondConfig := secondStep["config"].(map[string]any)
	if got := secondConfig["connector"]; got != "document_ingestion" {
		t.Fatalf("expected connector derived from integration, got: %#v", got)
	}
}

func TestNormalizeToBuilderASTYAML_NormalizesStepTypeAliases(t *testing.T) {
	input := `steps:
  - id: s1
    type: connector
    config:
      integration: postgres
`

	normalized, err := normalizeToBuilderASTYAML(input)
	if err != nil {
		t.Fatalf("normalizeToBuilderASTYAML returned error: %v", err)
	}

	var ast map[string]any
	if err := yaml.Unmarshal([]byte(normalized), &ast); err != nil {
		t.Fatalf("unmarshal normalized yaml: %v", err)
	}
	steps := ast["steps"].([]any)
	step := steps[0].(map[string]any)
	if got := step["type"]; got != "integration" {
		t.Fatalf("expected normalized step type integration, got: %#v", got)
	}
}

func TestNormalizeToBuilderASTYAML_IntegrationAliasToInput(t *testing.T) {
	input := `steps:
  - id: save_customer_to_postgres
    type: integration
    config:
      connector: postgres
      action: insert
      table: customer_onboarding
      data:
        email: "{{steps.s1.email}}"
      input_mapping:
        customer_id: "{{steps.s1.customer_id}}"
`

	normalized, err := normalizeToBuilderASTYAML(input)
	if err != nil {
		t.Fatalf("normalizeToBuilderASTYAML returned error: %v", err)
	}

	var ast map[string]any
	if err := yaml.Unmarshal([]byte(normalized), &ast); err != nil {
		t.Fatalf("unmarshal normalized yaml: %v", err)
	}
	steps := ast["steps"].([]any)
	step := steps[0].(map[string]any)
	cfg := step["config"].(map[string]any)
	inputCfg, ok := cfg["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected integration config.input to be present")
	}
	if got := inputCfg["table"]; got != "customer_onboarding" {
		t.Fatalf("expected input.table to be copied from config.table, got: %#v", got)
	}
	values, ok := inputCfg["values"].(map[string]any)
	if !ok {
		t.Fatalf("expected input.values to be present from config.data alias")
	}
	if got := values["email"]; got != "{{steps.s1.email}}" {
		t.Fatalf("expected input.values.email copied from data alias, got: %#v", got)
	}
	if got := inputCfg["customer_id"]; got != "{{steps.s1.customer_id}}" {
		t.Fatalf("expected input_mapping aliases merged into input, got: %#v", got)
	}
}

func TestNormalizeToBuilderASTYAML_AIComponentAgentAlias(t *testing.T) {
	input := `steps:
  - id: classify_doc
    type: agent
    config:
      ai_component_id: document_classifier
      input_mapping:
        doc_text: case.data.documents.latest.text
`

	normalized, err := normalizeToBuilderASTYAML(input)
	if err != nil {
		t.Fatalf("normalizeToBuilderASTYAML returned error: %v", err)
	}

	var ast map[string]any
	if err := yaml.Unmarshal([]byte(normalized), &ast); err != nil {
		t.Fatalf("unmarshal normalized yaml: %v", err)
	}
	steps := ast["steps"].([]any)
	step := steps[0].(map[string]any)
	if got := step["type"]; got != "ai_component" {
		t.Fatalf("expected normalized step type ai_component, got: %#v", got)
	}
	cfg := step["config"].(map[string]any)
	if got := cfg["component"]; got != "document_classifier" {
		t.Fatalf("expected component copied from ai_component_id, got: %#v", got)
	}
	if got := cfg["output_path"]; got != "case.data.ai.document_classifier" {
		t.Fatalf("expected default output path, got: %#v", got)
	}
	inputPaths, ok := cfg["input_paths"].(map[string]any)
	if !ok {
		t.Fatalf("expected input_paths to be present")
	}
	if got := inputPaths["doc_text"]; got != "case.data.documents.latest.text" {
		t.Fatalf("expected input_paths.doc_text copied from input_mapping, got: %#v", got)
	}
}

func TestNormalizeToBuilderASTYAML_ExtractionStepAlias(t *testing.T) {
	input := `steps:
  - id: extract_loan_application
    type: document_extraction
    config:
      document_path: case.data.attachments[0].vault_id
      schema: loan_application_pdf
      output_path: case.data.extracted
`

	normalized, err := normalizeToBuilderASTYAML(input)
	if err != nil {
		t.Fatalf("normalizeToBuilderASTYAML returned error: %v", err)
	}

	var ast map[string]any
	if err := yaml.Unmarshal([]byte(normalized), &ast); err != nil {
		t.Fatalf("unmarshal normalized yaml: %v", err)
	}
	steps := ast["steps"].([]any)
	step := steps[0].(map[string]any)
	if got := step["type"]; got != "extraction" {
		t.Fatalf("expected normalized step type extraction, got: %#v", got)
	}
}

func TestNormalizeToBuilderASTYAML_GoldenPromptFixtures(t *testing.T) {
	t.Parallel()

	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "ai_builder_golden")
	assertFixtureExists(t, fixtureDir, "prompt-a-assistant-raw.json")
	assertFixtureExists(t, fixtureDir, "prompt-b-assistant-raw.json")
	assertFixtureExists(t, fixtureDir, "prompt-a-normalized.json")
	assertFixtureExists(t, fixtureDir, "prompt-b-normalized.json")

	for _, tc := range []struct {
		name           string
		rawFixture     string
		oracleFixture  string
		thresholdToken string
	}{
		{
			name:           "prompt_a",
			rawFixture:     "prompt-a-assistant-raw.json",
			oracleFixture:  "prompt-a-normalized.json",
			thresholdToken: "0.80",
		},
		{
			name:           "prompt_b",
			rawFixture:     "prompt-b-assistant-raw.json",
			oracleFixture:  "prompt-b-normalized.json",
			thresholdToken: "0.70",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawYAML := loadRawFixtureYAML(t, filepath.Join(fixtureDir, tc.rawFixture))
			if !strings.Contains(rawYAML, tc.thresholdToken) {
				t.Fatalf("raw fixture must pin routing threshold %s", tc.thresholdToken)
			}

			firstNormalized, err := normalizeToBuilderASTYAML(rawYAML)
			if err != nil {
				t.Fatalf("normalizeToBuilderASTYAML returned error: %v", err)
			}
			secondNormalized, err := normalizeToBuilderASTYAML(firstNormalized)
			if err != nil {
				t.Fatalf("normalize(normalized) returned error: %v", err)
			}

			firstObj := mustYAMLObject(t, firstNormalized)
			secondObj := mustYAMLObject(t, secondNormalized)
			if !reflect.DeepEqual(firstObj, secondObj) {
				t.Fatalf("expected normalization idempotency for fixture %s", tc.rawFixture)
			}

			wantObj := loadNormalizedOracle(t, filepath.Join(fixtureDir, tc.oracleFixture))
			if !reflect.DeepEqual(firstObj, wantObj) {
				t.Fatalf("normalized output mismatch for fixture %s", tc.rawFixture)
			}
		})
	}
}

func TestNormalizeToBuilderASTYAML_RejectsUnknownStepTypeAlias(t *testing.T) {
	input := `steps:
  - id: bad
    type: magical_step
    config: {}`
	_, err := normalizeToBuilderASTYAML(input)
	if err == nil {
		t.Fatal("expected unknown step type alias to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown step type") {
		t.Fatalf("expected unknown step type error, got %v", err)
	}
}

func assertFixtureExists(t *testing.T, fixtureDir, filename string) {
	t.Helper()
	path := filepath.Join(fixtureDir, filename)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected fixture at %s: %v", path, err)
	}
}

func loadRawFixtureYAML(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw fixture %s: %v", path, err)
	}
	var payload struct {
		YAML string `json:"yaml"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode raw fixture %s: %v", path, err)
	}
	if strings.TrimSpace(payload.YAML) == "" {
		t.Fatalf("raw fixture %s missing yaml payload", path)
	}
	return payload.YAML
}

func loadNormalizedOracle(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read oracle fixture %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode oracle fixture %s: %v", path, err)
	}
	return out
}

func mustYAMLObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := yaml.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode yaml object: %v", err)
	}
	return out
}
