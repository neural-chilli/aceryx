package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type StepExecutor struct {
	db      *sql.DB
	manager *Manager
}

type StepConfig struct {
	ServerURL     string         `json:"server_url"`
	AuthType      string         `json:"auth_type"`
	AuthSecret    string         `json:"auth_secret"`
	AuthHeader    string         `json:"auth_header"`
	Tool          string         `json:"tool"`
	Arguments     map[string]any `json:"arguments"`
	OutputPath    string         `json:"output_path"`
	TimeoutMS     int            `json:"timeout_ms"`
	TimeoutSecond int            `json:"timeout_seconds"`
	Depth         int            `json:"depth"`
}

func NewStepExecutor(db *sql.DB, manager *Manager) *StepExecutor {
	return &StepExecutor{db: db, manager: manager}
}

func (e *StepExecutor) Execute(ctx context.Context, caseID uuid.UUID, _ string, raw json.RawMessage) (*engine.StepResult, error) {
	if e == nil || e.manager == nil {
		return nil, fmt.Errorf("mcp step executor not configured")
	}
	cfg := StepConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse mcp-client step config: %w", err)
	}
	if strings.TrimSpace(cfg.ServerURL) == "" || strings.TrimSpace(cfg.Tool) == "" {
		return nil, fmt.Errorf("server_url and tool are required")
	}
	ctxData, tenantID, err := e.loadCaseContext(ctx, caseID)
	if err != nil {
		return nil, err
	}
	resolvedArgsAny := connectors.ResolveTemplateAny(cfg.Arguments, ctxData)
	resolvedArgs, _ := resolvedArgsAny.(map[string]any)
	if resolvedArgs == nil {
		resolvedArgs = map[string]any{}
	}
	argsRaw, err := MarshalAny(resolvedArgs)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp arguments: %w", err)
	}
	timeoutMS := cfg.TimeoutMS
	if timeoutMS <= 0 && cfg.TimeoutSecond > 0 {
		timeoutMS = int((time.Duration(cfg.TimeoutSecond) * time.Second) / time.Millisecond)
	}
	invokeResult, err := e.manager.InvokeTool(ctx, InvokeRequest{
		TenantID:  tenantID,
		ServerURL: cfg.ServerURL,
		Auth: AuthConfig{
			Type:       cfg.AuthType,
			SecretRef:  cfg.AuthSecret,
			HeaderName: cfg.AuthHeader,
		},
		ToolName:  cfg.Tool,
		Arguments: argsRaw,
		Depth:     cfg.Depth,
		TimeoutMS: timeoutMS,
	})
	if err != nil {
		return nil, err
	}
	if invokeResult.IsError {
		return nil, errors.New(ToolErrorMessage(invokeResult))
	}
	outputRaw, err := json.Marshal(invokeResult)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp result: %w", err)
	}
	patch, err := buildCasePatch(cfg.OutputPath, invokeResult)
	if err != nil {
		return nil, err
	}
	return &engine.StepResult{
		Outcome:        "ok",
		Output:         outputRaw,
		WritesCaseData: len(patch) > 0,
		CaseDataPatch:  patch,
		AuditEventType: "mcp.invoked",
	}, nil
}

func (e *StepExecutor) loadCaseContext(ctx context.Context, caseID uuid.UUID) (map[string]any, uuid.UUID, error) {
	if e == nil || e.db == nil {
		return nil, uuid.Nil, fmt.Errorf("mcp step executor db not configured")
	}
	var (
		tenantID uuid.UUID
		caseData []byte
	)
	if err := e.db.QueryRowContext(ctx, `SELECT tenant_id, data FROM cases WHERE id = $1`, caseID).Scan(&tenantID, &caseData); err != nil {
		return nil, uuid.Nil, fmt.Errorf("load case context for mcp step: %w", err)
	}
	caseMap := map[string]any{}
	if len(caseData) > 0 {
		if err := json.Unmarshal(caseData, &caseMap); err != nil {
			return nil, uuid.Nil, fmt.Errorf("decode case data for mcp step: %w", err)
		}
	}
	return map[string]any{
		"case": map[string]any{"data": caseMap},
		"now":  time.Now().UTC().Format(time.RFC3339),
	}, tenantID, nil
}

func buildCasePatch(outputPath string, value any) (json.RawMessage, error) {
	path := strings.TrimSpace(outputPath)
	if path == "" {
		return nil, nil
	}
	path = strings.TrimPrefix(path, "case.data.")
	path = strings.TrimPrefix(path, "data.")
	path = strings.Trim(path, ".")
	if path == "" {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal mcp output for case patch: %w", err)
		}
		return json.RawMessage(raw), nil
	}
	parts := strings.Split(path, ".")
	root := map[string]any{}
	cur := root
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == len(parts)-1 {
			cur[p] = value
			continue
		}
		next := map[string]any{}
		cur[p] = next
		cur = next
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp case patch: %w", err)
	}
	return json.RawMessage(raw), nil
}
