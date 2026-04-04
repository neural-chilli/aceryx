package integration

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type testFixtures struct {
	TenantID    uuid.UUID
	PrincipalID uuid.UUID
	CaseTypeID  uuid.UUID
	CaseType    string
	WorkflowID  uuid.UUID
	WorkflowVer int
}

type testFixturesBuilder struct {
	t           *testing.T
	ctx         context.Context
	db          *sql.DB
	tenantID    uuid.UUID
	principal   uuid.UUID
	caseType    string
	caseTypeID  uuid.UUID
	workflowID  uuid.UUID
	workflowVer int
}

func newFixtures(t *testing.T, ctx context.Context, db *sql.DB) *testFixturesBuilder {
	t.Helper()
	return &testFixturesBuilder{t: t, ctx: ctx, db: db}
}

func (b *testFixturesBuilder) WithTenant(slug string) *testFixturesBuilder {
	b.t.Helper()
	b.tenantID, b.principal = seedTenantAndPrincipal(b.t, b.ctx, b.db, slug)
	return b
}

func (b *testFixturesBuilder) WithCaseType(name string, schema cases.CaseTypeSchema) *testFixturesBuilder {
	b.t.Helper()
	if b.tenantID == uuid.Nil || b.principal == uuid.Nil {
		b.t.Fatal("WithTenant must be called before WithCaseType")
	}
	ctSvc := cases.NewCaseTypeService(b.db)
	ct, schemaErrs, err := ctSvc.RegisterCaseType(b.ctx, b.tenantID, b.principal, name, schema)
	if err != nil {
		b.t.Fatalf("register case type: %v", err)
	}
	if len(schemaErrs) > 0 {
		b.t.Fatalf("unexpected schema validation errors: %+v", schemaErrs)
	}
	b.caseType = ct.Name
	b.caseTypeID = ct.ID
	return b
}

func (b *testFixturesBuilder) WithPublishedWorkflow(ast engine.WorkflowAST) *testFixturesBuilder {
	b.t.Helper()
	if b.caseType == "" {
		b.t.Fatal("WithCaseType must be called before WithPublishedWorkflow")
	}
	b.workflowID, b.workflowVer = seedPublishedWorkflow(b.t, b.ctx, b.db, b.tenantID, b.principal, b.caseType, ast)
	return b
}

func (b *testFixturesBuilder) Build() testFixtures {
	b.t.Helper()
	return testFixtures{
		TenantID:    b.tenantID,
		PrincipalID: b.principal,
		CaseTypeID:  b.caseTypeID,
		CaseType:    b.caseType,
		WorkflowID:  b.workflowID,
		WorkflowVer: b.workflowVer,
	}
}
