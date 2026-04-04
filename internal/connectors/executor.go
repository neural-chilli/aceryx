package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/observability"
)

type Executor struct {
	db       *sql.DB
	registry *Registry
	secrets  SecretStore
}

type StepConfig struct {
	Connector      string            `json:"connector"`
	Action         string            `json:"action"`
	Auth           map[string]string `json:"auth"`
	Input          map[string]any    `json:"input"`
	TimeoutSeconds int               `json:"timeout_seconds"`
}

func NewExecutor(db *sql.DB, registry *Registry, secrets SecretStore) *Executor {
	return &Executor{db: db, registry: registry, secrets: secrets}
}

func (e *Executor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, raw json.RawMessage) (*engine.StepResult, error) {
	start := time.Now()
	cfg := StepConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse integration step config: %w", err)
	}
	if cfg.Connector == "" || cfg.Action == "" {
		return nil, errors.New("connector and action are required")
	}

	action, ok := e.registry.GetAction(cfg.Connector, cfg.Action)
	if !ok {
		return nil, fmt.Errorf("connector action not found: %s/%s", cfg.Connector, cfg.Action)
	}

	caseCtx, tenantID, err := e.loadCaseContext(ctx, caseID)
	if err != nil {
		return nil, err
	}

	resolvedAuth := make(map[string]string, len(cfg.Auth))
	for k, v := range cfg.Auth {
		resolvedAuth[k] = ResolveTemplateString(v, caseCtx)
	}

	if connector, ok := e.registry.Get(cfg.Connector); ok {
		for _, field := range connector.Auth().Fields {
			if _, exists := resolvedAuth[field.Key]; exists && resolvedAuth[field.Key] != "" {
				continue
			}
			if e.secrets == nil {
				continue
			}
			value, gerr := e.secrets.Get(ctx, tenantID, field.Key)
			if gerr == nil && value != "" {
				resolvedAuth[field.Key] = value
			}
		}
	}

	resolvedInputAny := ResolveTemplateAny(cfg.Input, caseCtx)
	resolvedInput, _ := resolvedInputAny.(map[string]any)
	if resolvedInput == nil {
		resolvedInput = map[string]any{}
	}
	resolvedInput["_case_id"] = caseID.String()
	resolvedInput["_step_id"] = stepID
	resolvedInput["_tenant_id"] = tenantID.String()

	timeout := 30 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := action.Execute(actx, resolvedAuth, resolvedInput)
	if err != nil {
		status := "error"
		if errors.Is(err, context.DeadlineExceeded) {
			status = "timeout"
		}
		observability.ConnectorCallsTotal.WithLabelValues(tenantID.String(), cfg.Connector, cfg.Action, status).Inc()
		observability.ConnectorDurationSeconds.WithLabelValues(tenantID.String(), cfg.Connector).Observe(time.Since(start).Seconds())
		slog.ErrorContext(ctx, "connector call failed",
			append(observability.RequestAttrs(ctx),
				"case_id", caseID.String(),
				"step_id", stepID,
				"connector", cfg.Connector,
				"action", cfg.Action,
				"error", err,
			)...,
		)
		return nil, err
	}
	observability.ConnectorCallsTotal.WithLabelValues(tenantID.String(), cfg.Connector, cfg.Action, "success").Inc()
	observability.ConnectorDurationSeconds.WithLabelValues(tenantID.String(), cfg.Connector).Observe(time.Since(start).Seconds())
	slog.InfoContext(ctx, "connector call completed",
		append(observability.RequestAttrs(ctx),
			"case_id", caseID.String(),
			"step_id", stepID,
			"connector", cfg.Connector,
			"action", cfg.Action,
			"duration_ms", time.Since(start).Milliseconds(),
		)...,
	)
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal connector action result: %w", err)
	}
	return &engine.StepResult{Output: payload}, nil
}

func (e *Executor) loadCaseContext(ctx context.Context, caseID uuid.UUID) (map[string]any, uuid.UUID, error) {
	var (
		tenantID    uuid.UUID
		caseNumber  string
		caseStatus  string
		caseDataRaw []byte
		brandingRaw []byte
	)
	err := e.db.QueryRowContext(ctx, `
SELECT c.tenant_id, c.case_number, c.status, c.data, t.branding
FROM cases c
JOIN tenants t ON t.id = c.tenant_id
WHERE c.id = $1
`, caseID).Scan(&tenantID, &caseNumber, &caseStatus, &caseDataRaw, &brandingRaw)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("load case context: %w", err)
	}
	if expectedTenant := observability.TenantIDFromContext(ctx); expectedTenant != "" && expectedTenant != tenantID.String() {
		return nil, uuid.Nil, fmt.Errorf("load case context: tenant mismatch for case %s", caseID)
	}

	caseData := map[string]any{}
	_ = json.Unmarshal(caseDataRaw, &caseData)
	branding := map[string]any{}
	_ = json.Unmarshal(brandingRaw, &branding)

	steps := map[string]any{}
	rows, err := e.db.QueryContext(ctx, `
SELECT cs.step_id, COALESCE(cs.result, '{}'::jsonb)
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE cs.case_id = $1
  AND c.tenant_id = $2
`, caseID, tenantID)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("load step results for template context: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var stepID string
		var raw []byte
		if err := rows.Scan(&stepID, &raw); err != nil {
			return nil, uuid.Nil, fmt.Errorf("scan step context row: %w", err)
		}
		result := map[string]any{}
		_ = json.Unmarshal(raw, &result)
		steps[stepID] = map[string]any{"result": result}
	}
	if err := rows.Err(); err != nil {
		return nil, uuid.Nil, fmt.Errorf("iterate step context rows: %w", err)
	}

	caseMap := map[string]any{
		"id":          caseID.String(),
		"case_number": caseNumber,
		"status":      caseStatus,
		"data":        caseData,
		"steps":       steps,
	}
	for k, v := range caseData {
		caseMap[k] = v
	}

	templateContext := map[string]any{
		"case":   caseMap,
		"tenant": map[string]any{"branding": branding},
		"now":    time.Now().UTC().Format(time.RFC3339),
	}
	if e.secrets != nil {
		templateContext["__secret_resolver"] = func(key string) string {
			v, gerr := e.secrets.Get(ctx, tenantID, key)
			if gerr != nil {
				return ""
			}
			return v
		}
	}
	return templateContext, tenantID, nil
}
