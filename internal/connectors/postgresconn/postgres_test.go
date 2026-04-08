package postgresconn

import "testing"

func TestBuildWhereSQL(t *testing.T) {
	where := map[string]any{
		"status":     "active",
		"age":        map[string]any{"op": "gte", "value": 21},
		"deleted_at": nil,
	}
	query, args, err := buildWhereSQL(where, 1)
	if err != nil {
		t.Fatalf("buildWhereSQL returned error: %v", err)
	}
	if query == "" {
		t.Fatal("expected non-empty where query")
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}

func TestReadColumnListRejectsInvalidIdentifier(t *testing.T) {
	_, err := readColumnList([]any{"ok", "drop table users"}, nil)
	if err == nil {
		t.Fatal("expected invalid identifier error")
	}
}

func TestActionsIncludeStructuredCRUDAndTemplates(t *testing.T) {
	actions := New().Actions()
	keys := map[string]bool{}
	for _, action := range actions {
		keys[action.Key] = true
	}
	expected := []string{"select", "insert", "update", "delete", "upsert", "query_template", "exec_template"}
	for _, key := range expected {
		if !keys[key] {
			t.Fatalf("missing action %q", key)
		}
	}
}
