package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type componentStepConfig struct {
	ComponentID  string            `json:"component"`
	InputPaths   map[string]string `json:"input_paths"`
	OutputPath   string            `json:"output_path"`
	ConfigValues map[string]string `json:"config_values"`
}

type StepExecutor struct {
	db       *sql.DB
	executor *ComponentExecutor
}

func NewStepExecutor(db *sql.DB, executor *ComponentExecutor) *StepExecutor {
	return &StepExecutor{db: db, executor: executor}
}

func (s *StepExecutor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, config json.RawMessage) (*engine.StepResult, error) {
	if s == nil || s.executor == nil {
		return nil, fmt.Errorf("ai component step executor not configured")
	}
	cfg := componentStepConfig{}
	if len(config) > 0 {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, fmt.Errorf("decode ai component step config: %w", err)
		}
	}
	if strings.TrimSpace(cfg.ComponentID) == "" {
		return nil, fmt.Errorf("ai component step config missing component")
	}
	tenantID, err := s.lookupTenantID(ctx, caseID)
	if err != nil {
		return nil, err
	}
	out, err := s.executor.Execute(ctx, ComponentExecRequest{
		TenantID:     tenantID,
		CaseID:       caseID,
		StepID:       stepID,
		ComponentID:  cfg.ComponentID,
		InputPaths:   cfg.InputPaths,
		OutputPath:   cfg.OutputPath,
		ConfigValues: cfg.ConfigValues,
	})
	if err != nil {
		return nil, err
	}
	if out.Status == "escalated" {
		return nil, engine.ErrStepAwaitingReview
	}
	return &engine.StepResult{
		Output:         out.Output,
		WritesCaseData: len(out.MergePatch) > 0,
		CaseDataPatch:  out.MergePatch,
		ExecutionEvent: out.Event,
		AuditEventType: "ai_component.executed",
	}, nil
}

func (s *StepExecutor) lookupTenantID(ctx context.Context, caseID uuid.UUID) (uuid.UUID, error) {
	if s == nil || s.db == nil {
		return uuid.Nil, fmt.Errorf("step executor db not configured")
	}
	var tenantID uuid.UUID
	if err := s.db.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1`, caseID).Scan(&tenantID); err != nil {
		return uuid.Nil, fmt.Errorf("resolve case tenant: %w", err)
	}
	return tenantID, nil
}
