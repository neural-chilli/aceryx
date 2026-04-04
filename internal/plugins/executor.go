package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type StepExecutor struct {
	db      *sql.DB
	runtime PluginRuntime
}

type StepConfig struct {
	Plugin  string          `json:"plugin"`
	Input   json.RawMessage `json:"input"`
	Timeout int             `json:"timeout_seconds"`
}

func NewStepExecutor(db *sql.DB, runtime PluginRuntime) *StepExecutor {
	return &StepExecutor{db: db, runtime: runtime}
}

func (e *StepExecutor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, raw json.RawMessage) (*engine.StepResult, error) {
	if e.runtime == nil {
		return nil, fmt.Errorf("plugin runtime not configured")
	}
	cfg := StepConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse plugin step config: %w", err)
	}
	ref, err := ParsePluginRefStrict(cfg.Plugin)
	if err != nil {
		return nil, err
	}
	tenantID, payload, err := e.loadCasePayload(ctx, caseID)
	if err != nil {
		return nil, err
	}
	input := cfg.Input
	if len(input) == 0 {
		input = payload
	}
	timeout := 30 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	out, err := e.runtime.ExecuteStep(ctx, ref, StepInput{
		TenantID: tenantID,
		CaseID:   caseID,
		StepID:   stepID,
		Data:     input,
		Timeout:  timeout,
	})
	if err != nil {
		return nil, err
	}
	if out.Status == "error" {
		return nil, errors.New(out.Error)
	}
	return &engine.StepResult{
		Outcome: "ok",
		Output:  out.Output,
	}, nil
}

func (e *StepExecutor) loadCasePayload(ctx context.Context, caseID uuid.UUID) (uuid.UUID, json.RawMessage, error) {
	var (
		tenantID uuid.UUID
		raw      []byte
	)
	err := e.db.QueryRowContext(ctx, `SELECT tenant_id, data FROM cases WHERE id = $1`, caseID).Scan(&tenantID, &raw)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("load plugin step case context: %w", err)
	}
	return tenantID, json.RawMessage(raw), nil
}
