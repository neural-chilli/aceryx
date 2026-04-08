package extraction

import (
	"encoding/json"
	"testing"
)

func TestDecodeStepConfig_AcceptsCanonicalKeys(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"document_ref": "case.data.documents.application_pdf",
		"schema_name":  "loan_application_pdf",
		"output_path":  "case.data.extracted.loan_application",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	cfg, err := decodeStepConfig(raw)
	if err != nil {
		t.Fatalf("decodeStepConfig returned error: %v", err)
	}
	if cfg.DocumentPath != "case.data.documents.application_pdf" {
		t.Fatalf("expected document_path fallback from document_ref, got %q", cfg.DocumentPath)
	}
	if cfg.Schema != "loan_application_pdf" {
		t.Fatalf("expected schema fallback from schema_name, got %q", cfg.Schema)
	}
}

func TestDecodeStepConfig_AcceptsSchemaIDFallback(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"document_path": "case.data.documents.application_pdf",
		"schema_id":     "schema-uuid-placeholder",
		"output_path":   "case.data.extracted.loan_application",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	cfg, err := decodeStepConfig(raw)
	if err != nil {
		t.Fatalf("decodeStepConfig returned error: %v", err)
	}
	if cfg.Schema != "schema-uuid-placeholder" {
		t.Fatalf("expected schema fallback from schema_id, got %q", cfg.Schema)
	}
}
