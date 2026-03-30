package reports

import (
	"strings"
	"testing"
)

func TestValidateReportSQL_AST(t *testing.T) {
	inspector := NewSQLInspector()

	t.Run("valid select accepted", func(t *testing.T) {
		if err := inspector.Validate(`SELECT case_type, COUNT(*) FROM mv_report_cases GROUP BY case_type`); err != nil {
			t.Fatalf("expected valid SQL, got %v", err)
		}
	})

	t.Run("dml rejected", func(t *testing.T) {
		if err := inspector.Validate(`DELETE FROM mv_report_cases`); err == nil {
			t.Fatal("expected DELETE to be rejected")
		}
	})

	t.Run("unapproved table rejected", func(t *testing.T) {
		if err := inspector.Validate(`SELECT * FROM cases`); err == nil {
			t.Fatal("expected raw table to be rejected")
		}
	})

	t.Run("pg catalog rejected", func(t *testing.T) {
		if err := inspector.Validate(`SELECT * FROM pg_catalog.pg_class`); err == nil {
			t.Fatal("expected pg catalog reference to be rejected")
		}
	})

	t.Run("unapproved function rejected", func(t *testing.T) {
		if err := inspector.Validate(`SELECT pg_sleep(1) FROM mv_report_cases`); err == nil {
			t.Fatal("expected unapproved function to be rejected")
		}
	})

	t.Run("unparseable sql rejected", func(t *testing.T) {
		if err := inspector.Validate(`SELECT FROM`); err == nil {
			t.Fatal("expected parse failure")
		}
	})
}

func TestScopeToTenant_ASTRewriteContract(t *testing.T) {
	inspector := NewSQLInspector()

	sqlJoin := `SELECT c.case_type, COUNT(*) FROM mv_report_cases c JOIN mv_report_steps s ON s.case_id = c.case_id GROUP BY c.case_type`
	scopedJoin, err := inspector.ScopeToTenant(sqlJoin)
	if err != nil {
		t.Fatalf("scope join query: %v", err)
	}
	if strings.Count(strings.ToLower(scopedJoin), "tenant_id = $1") < 2 {
		t.Fatalf("expected tenant filter for both views in join: %s", scopedJoin)
	}

	sqlGroup := `SELECT case_type, COUNT(*) AS total FROM mv_report_cases GROUP BY case_type`
	scopedGroup, err := inspector.ScopeToTenant(sqlGroup)
	if err != nil {
		t.Fatalf("scope group query: %v", err)
	}
	if !strings.Contains(strings.ToLower(scopedGroup), "tenant_id = $1") {
		t.Fatalf("expected tenant filter before aggregation: %s", scopedGroup)
	}
	if !strings.Contains(strings.ToLower(scopedGroup), "limit $2") {
		t.Fatalf("expected top-level limit injection: %s", scopedGroup)
	}
}

func TestBuildPrompt_NoDataLeak(t *testing.T) {
	views := []ViewSchema{{
		ViewName:    "mv_report_cases",
		Description: "Case summary",
		Columns: []ViewSchemaColumn{
			{Name: "case_number", Type: "text", Description: "Human readable case number"},
		},
	}}
	prompt := BuildPrompt(views, "How many completed cases last month?")
	if !strings.Contains(prompt, "mv_report_cases") || !strings.Contains(prompt, "How many completed cases last month?") {
		t.Fatalf("prompt missing expected schema/question context: %s", prompt)
	}
	if strings.Contains(strings.ToLower(prompt), `"rows"`) {
		t.Fatalf("prompt should not include customer result data: %s", prompt)
	}
}
