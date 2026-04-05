package ai

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/llm"
)

type fakeStepState struct {
	tenantID uuid.UUID
}

type fakeStepDriver struct{}

type fakeStepConn struct{ state *fakeStepState }

type fakeStepRows struct {
	cols []string
	vals [][]driver.Value
	i    int
}

var fakeStepDrivers sync.Map

func init() {
	sql.Register("fakestep", fakeStepDriver{})
}

func (d fakeStepDriver) Open(name string) (driver.Conn, error) {
	v, ok := fakeStepDrivers.Load(name)
	if !ok {
		return nil, errors.New("missing fake step state")
	}
	return &fakeStepConn{state: v.(*fakeStepState)}, nil
}

func (c *fakeStepConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (c *fakeStepConn) Close() error              { return nil }
func (c *fakeStepConn) Begin() (driver.Tx, error) { return nil, errors.New("not implemented") }
func (c *fakeStepConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	return &fakeStepRows{cols: []string{"tenant_id"}, vals: [][]driver.Value{{c.state.tenantID.String()}}, i: -1}, nil
}

func (r *fakeStepRows) Columns() []string { return r.cols }
func (r *fakeStepRows) Close() error      { return nil }
func (r *fakeStepRows) Next(dest []driver.Value) error {
	r.i++
	if r.i >= len(r.vals) {
		return io.EOF
	}
	copy(dest, r.vals[r.i])
	return nil
}

func openFakeStepDB(t *testing.T, tenantID uuid.UUID) *sql.DB {
	t.Helper()
	name := t.Name()
	fakeStepDrivers.Store(name, &fakeStepState{tenantID: tenantID})
	t.Cleanup(func() { fakeStepDrivers.Delete(name) })
	db, err := sql.Open("fakestep", name)
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStepExecutorRoutesAIComponentAndEscalationWaiting(t *testing.T) {
	tenantID := uuid.New()
	caseID := uuid.New()
	db := openFakeStepDB(t, tenantID)

	reg := NewComponentRegistry(nil)
	reg.global["c"] = &AIComponentDef{
		ID: "c", DisplayLabel: "C", Category: "AI", Tier: TierCommercial,
		InputSchema:  json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"confidence":{"type":"number"}},"required":["confidence"]}`),
		SystemPrompt: "sys", UserPromptTmpl: "{{.Input.x}}", ModelHints: ModelHints{PreferredSize: "small", MaxTokens: 10},
		Confidence: &ConfidenceConfig{FieldPath: "confidence", AutoAcceptAbove: 0.9, EscalateBelow: 0.6},
	}
	ce := NewComponentExecutor(
		&mockLLM{responses: []llm.ChatResponse{{Content: `{"confidence":0.4}`}}},
		&mockCaseStore{record: CaseRecord{ID: caseID, TenantID: tenantID, CaseType: "support", Data: map[string]any{"x": "hello"}}},
		&mockTaskStore{},
		reg,
	)
	exec := NewStepExecutor(db, ce)
	cfg := json.RawMessage(`{"component":"c","input_paths":{"x":"x"},"output_path":"out"}`)
	_, err := exec.Execute(context.Background(), caseID, "step-1", cfg)
	if !errors.Is(err, engine.ErrStepAwaitingReview) {
		t.Fatalf("expected ErrStepAwaitingReview, got %v", err)
	}
}
