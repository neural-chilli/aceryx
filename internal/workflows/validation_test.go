package workflows

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateWorkflowAST_AgentConfigContract(t *testing.T) {
	t.Run("accepts canonical agent config", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":   "risk",
					"type": "agent",
					"config": map[string]any{
						"context": []map[string]any{
							{"source": "case", "fields": []string{"applicant", "loan"}},
						},
						"prompt_template": "risk_v1",
						"output_schema": map[string]any{
							"score": map[string]any{"type": "number", "min": 0, "max": 100},
						},
						"on_low_confidence": "escalate_to_human",
					},
				},
			},
		})
		if err := validateWorkflowAST(raw); err != nil {
			t.Fatalf("expected valid agent ast, got %v", err)
		}
	})

	t.Run("rejects legacy context_sources key", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":   "risk",
					"type": "agent",
					"config": map[string]any{
						"prompt_template": "risk_v1",
						"context_sources": []map[string]any{
							{"source": "case"},
						},
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "context_sources") {
			t.Fatalf("expected context_sources validation error, got %v", err)
		}
	})

	t.Run("rejects string output_schema", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":   "risk",
					"type": "agent",
					"config": map[string]any{
						"prompt_template": "risk_v1",
						"output_schema":   "{\"score\":{\"type\":\"number\"}}",
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "output_schema") {
			t.Fatalf("expected output_schema validation error, got %v", err)
		}
	})

	t.Run("rejects unsupported on_low_confidence value", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":   "risk",
					"type": "agent",
					"config": map[string]any{
						"prompt_template":   "risk_v1",
						"on_low_confidence": "human_review",
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "on_low_confidence") {
			t.Fatalf("expected on_low_confidence validation error, got %v", err)
		}
	})

	t.Run("rejects non-array context", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":   "risk",
					"type": "agent",
					"config": map[string]any{
						"prompt_template": "risk_v1",
						"context":         map[string]any{"source": "case"},
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "context") {
			t.Fatalf("expected context validation error, got %v", err)
		}
	})

	t.Run("rejects missing integration required config", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":     "fetch",
					"type":   "integration",
					"config": map[string]any{"connector": "http"},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "integration requires connector and action") {
			t.Fatalf("expected integration required-config error, got %v", err)
		}
	})

	t.Run("rejects dangling outcome target", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":       "route",
					"type":     "rule",
					"outcomes": map[string]any{"approve": []string{"missing_step"}},
					"config": map[string]any{
						"outcomes": []map[string]any{
							{"name": "approve", "condition": "true", "target": "missing_step"},
						},
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "targets unknown step") {
			t.Fatalf("expected dangling outcome target error, got %v", err)
		}
	})

	t.Run("rejects invalid step condition expression", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":         "h1",
					"type":       "human_task",
					"depends_on": []string{},
					"condition":  "case.data.amount >",
					"config": map[string]any{
						"assign_to_role": "case_worker",
						"form_schema": map[string]any{
							"fields": []any{},
						},
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "condition is invalid") {
			t.Fatalf("expected condition validation error, got %v", err)
		}
	})

	t.Run("rejects non-boolean rule outcome expression", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":       "route",
					"type":     "rule",
					"outcomes": map[string]any{"approve": []string{"done"}},
					"config": map[string]any{
						"outcomes": []map[string]any{
							{"name": "approve", "condition": `"yes"`, "target": "done"},
						},
					},
				},
				{
					"id":   "done",
					"type": "notification",
					"config": map[string]any{
						"channel": "email",
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "must evaluate to boolean") {
			t.Fatalf("expected non-boolean rule condition error, got %v", err)
		}
	})

	t.Run("accepts valid mixed workflow", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":         "review",
					"type":       "human_task",
					"depends_on": []string{},
					"condition":  "true",
					"config": map[string]any{
						"assign_to_role": "case_worker",
						"form_schema": map[string]any{
							"fields": []any{},
						},
					},
				},
				{
					"id":         "extract",
					"type":       "ai_component",
					"depends_on": []string{"review"},
					"config": map[string]any{
						"component":     "document_classifier",
						"input_paths":   map[string]any{"doc_text": "case.data.text"},
						"config_values": map[string]any{"threshold": "0.7"},
						"output_path":   "case.data.ai.document_classifier",
					},
				},
				{
					"id":         "call_api",
					"type":       "integration",
					"depends_on": []string{"extract"},
					"config": map[string]any{
						"connector": "http",
						"action":    "request",
					},
				},
				{
					"id":         "notify",
					"type":       "notification",
					"depends_on": []string{"call_api"},
					"config": map[string]any{
						"channel": "email",
					},
				},
			},
		})
		if err := validateWorkflowAST(raw); err != nil {
			t.Fatalf("expected valid mixed workflow ast, got %v", err)
		}
	})

	t.Run("accepts extraction step with thresholds and review routing", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":   "extract",
					"type": "extraction",
					"config": map[string]any{
						"document_path":         "case.data.attachments[0].vault_id",
						"schema":                "loan_application_pdf",
						"model":                 "gpt-5.4",
						"auto_accept_threshold": 0.85,
						"review_threshold":      0.3,
						"output_path":           "case.data.extracted",
						"on_review":             map[string]any{"task_type": "extraction_review", "assignee_role": "underwriter", "sla_hours": 4},
						"on_reject":             map[string]any{"goto": "manual_data_entry"},
					},
				},
			},
		})
		if err := validateWorkflowAST(raw); err != nil {
			t.Fatalf("expected valid extraction workflow ast, got %v", err)
		}
	})

	t.Run("rejects extraction threshold ranges", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":   "extract",
					"type": "extraction",
					"config": map[string]any{
						"document_path":         "case.data.attachments[0].vault_id",
						"schema":                "loan_application_pdf",
						"output_path":           "case.data.extracted",
						"auto_accept_threshold": 0.2,
						"review_threshold":      0.4,
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "review_threshold cannot exceed auto_accept_threshold") {
			t.Fatalf("expected threshold ordering error, got %v", err)
		}
	})

	t.Run("rejects extraction on_review SLA <= 0", func(t *testing.T) {
		raw := mustJSON(t, map[string]any{
			"steps": []map[string]any{
				{
					"id":   "extract",
					"type": "extraction",
					"config": map[string]any{
						"document_path": "case.data.attachments[0].vault_id",
						"schema":        "loan_application_pdf",
						"output_path":   "case.data.extracted",
						"on_review":     map[string]any{"sla_hours": 0},
					},
				},
			},
		})
		err := validateWorkflowAST(raw)
		if err == nil || !strings.Contains(err.Error(), "on_review.sla_hours") {
			t.Fatalf("expected on_review.sla_hours validation error, got %v", err)
		}
	})
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}
