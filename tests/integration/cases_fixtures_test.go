package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/notify"
)

func seedTenantAndPrincipal(t *testing.T, ctx context.Context, db *sql.DB, slug string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	var tenantID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO tenants (name, slug, branding, terminology, settings)
VALUES ($1, $2, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb)
RETURNING id
`, "Tenant "+slug, slug).Scan(&tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	var principalID uuid.UUID
	err = db.QueryRowContext(ctx, `
INSERT INTO principals (tenant_id, type, name, email, status)
VALUES ($1, 'human', $2, $3, 'active')
RETURNING id
`, tenantID, "Principal "+slug, slug+"@example.com").Scan(&principalID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}
	return tenantID, principalID
}

func seedPublishedWorkflow(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, caseTypeName string, ast engine.WorkflowAST) (uuid.UUID, int) {
	t.Helper()
	var workflowID uuid.UUID
	err := db.QueryRowContext(ctx, `
INSERT INTO workflows (tenant_id, name, case_type, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id
`, tenantID, "workflow-"+uuid.NewString()[:8], caseTypeName, principalID).Scan(&workflowID)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	rawAST, err := json.Marshal(ast)
	if err != nil {
		t.Fatalf("marshal ast: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workflow_versions (workflow_id, version, status, ast, yaml_source, created_by, published_at)
VALUES ($1, 1, 'published', $2::jsonb, '', $3, now())
`, workflowID, string(rawAST), principalID); err != nil {
		t.Fatalf("insert workflow version: %v", err)
	}
	return workflowID, 1
}

func seedAdditionalCaseType(t *testing.T, ctx context.Context, db *sql.DB, tenantID, principalID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var existing uuid.UUID
	if err := db.QueryRowContext(ctx, `
SELECT id
FROM case_types
WHERE tenant_id = $1 AND name = $2 AND version = 1
LIMIT 1
`, tenantID, name).Scan(&existing); err == nil {
		return existing
	}
	rawSchema, err := json.Marshal(testCaseSchema())
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var id uuid.UUID
	err = db.QueryRowContext(ctx, `
INSERT INTO case_types (tenant_id, name, version, schema, status, created_by)
VALUES ($1, $2, 1, $3::jsonb, 'active', $4)
RETURNING id
`, tenantID, name, string(rawSchema), principalID).Scan(&id)
	if err != nil {
		t.Fatalf("insert additional case type: %v", err)
	}
	return id
}

func testCaseSchema() cases.CaseTypeSchema {
	minAmount := 5000.0
	maxAmount := 500000.0
	minName := 2
	maxName := 120
	return cases.CaseTypeSchema{Fields: map[string]cases.SchemaField{
		"applicant": {
			Type: "object",
			Properties: map[string]cases.SchemaField{
				"company_name":        {Type: "string", Required: true, MinLength: &minName, MaxLength: &maxName},
				"registration_number": {Type: "string", Pattern: "^[0-9]{8}$"},
			},
		},
		"loan": {
			Type: "object",
			Properties: map[string]cases.SchemaField{
				"amount":      {Type: "number", Required: true, Min: &minAmount, Max: &maxAmount},
				"term_months": {Type: "integer"},
			},
		},
		"decision": {
			Type:   "string",
			Source: "agent",
			Enum:   []interface{}{"pending", "approved", "rejected"},
		},
	}}
}

func waitAndLoadCaseSteps(t *testing.T, ctx context.Context, db *sql.DB, caseID uuid.UUID, expected int) []cases.CaseStep {
	t.Helper()
	var out []cases.CaseStep
	waitForCondition(t, 4*time.Second, 50*time.Millisecond, func() bool {
		rows, err := db.QueryContext(ctx, `
SELECT id, step_id, state, started_at, completed_at, result, events, error, assigned_to, sla_deadline, retry_count, draft_data, metadata
FROM case_steps
WHERE case_id=$1
ORDER BY step_id
`, caseID)
		if err != nil {
			t.Fatalf("query steps: %v", err)
		}
		current := make([]cases.CaseStep, 0)
		for rows.Next() {
			var st cases.CaseStep
			if err := rows.Scan(&st.ID, &st.StepID, &st.State, &st.StartedAt, &st.CompletedAt, &st.Result, &st.Events, &st.Error, &st.AssignedTo, &st.SLADeadline, &st.RetryCount, &st.DraftData, &st.Metadata); err != nil {
				_ = rows.Close()
				t.Fatalf("scan steps: %v", err)
			}
			current = append(current, st)
		}
		_ = rows.Close()
		if len(current) >= expected {
			out = current
			return true
		}
		return false
	}, "steps did not reach expected count in time")
	return out
}

type stubCaseEngine struct {
	cancelCalled bool
	cancelReason string
}

type caseNotifySpy struct {
	events []notify.NotifyEvent
}

func (s *caseNotifySpy) Notify(_ context.Context, event notify.NotifyEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubCaseEngine) EvaluateDAG(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (s *stubCaseEngine) CancelCase(_ context.Context, _ uuid.UUID, _ uuid.UUID, reason string) error {
	s.cancelCalled = true
	s.cancelReason = reason
	return nil
}

func intPtr(v int) *int { return &v }
