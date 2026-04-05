package agentic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/agentic/invokers"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/mcp"
	"github.com/neural-chilli/aceryx/internal/plugins"
	"github.com/neural-chilli/aceryx/internal/rag"
)

type StepExecutor struct {
	db            *sql.DB
	runner        AgenticRunner
	traceStore    TraceStore
	llm           LLMManager
	tasks         taskCreator
	pluginRuntime invokers.PluginRuntime
	mcpManager    invokers.MCPManager
	ragSearch     *rag.SearchService
}

func NewStepExecutor(
	db *sql.DB,
	runner AgenticRunner,
	traceStore TraceStore,
	llm LLMManager,
	tasks taskCreator,
	pluginRuntime invokers.PluginRuntime,
	mcpManager invokers.MCPManager,
	ragSearch *rag.SearchService,
) *StepExecutor {
	if runner == nil {
		runner = NewRunner()
	}
	if traceStore == nil {
		traceStore = NewPostgresTraceStore(db)
	}
	return &StepExecutor{
		db:            db,
		runner:        runner,
		traceStore:    traceStore,
		llm:           llm,
		tasks:         tasks,
		pluginRuntime: pluginRuntime,
		mcpManager:    mcpManager,
		ragSearch:     ragSearch,
	}
}

func (s *StepExecutor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, configRaw json.RawMessage) (*engine.StepResult, error) {
	if s == nil || s.db == nil || s.runner == nil || s.llm == nil {
		return nil, fmt.Errorf("agentic step executor not configured")
	}
	var cfg AgenticStepConfig
	if len(configRaw) > 0 {
		if err := json.Unmarshal(configRaw, &cfg); err != nil {
			return nil, fmt.Errorf("parse agentic config: %w", err)
		}
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	caseInfo, err := s.loadCaseInfo(ctx, caseID)
	if err != nil {
		return nil, err
	}
	manifest, err := s.buildManifest(ctx, caseInfo.TenantID, caseID, cfg)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(caseInfo.Model)
	if model == "" {
		model = "default"
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Limits.Timeout)
	defer cancel()
	runResult, err := s.runner.Run(runCtx, RunConfig{
		TenantID:     caseInfo.TenantID,
		CaseID:       caseID,
		StepID:       stepID,
		InstanceID:   caseInfo.InstanceID,
		Goal:         cfg.Goal,
		CaseData:     caseInfo.CaseData,
		ToolManifest: manifest,
		Limits:       cfg.Limits,
		OutputSchema: cfg.OutputSchema,
		LLMAdapter:   s.llm,
		TraceStore:   s.traceStore,
		Model:        model,
	})
	if err != nil {
		return nil, err
	}
	if runResult.Status == "error" {
		return nil, fmt.Errorf("agentic reasoning failed")
	}

	if runResult.Confidence != nil && *runResult.Confidence < cfg.Escalation.ConfidenceThreshold {
		events := []*ReasoningEvent(nil)
		if cfg.Escalation.IncludeTrace {
			events, _ = s.traceStore.GetEvents(ctx, runResult.TraceID)
		}
		if err := createEscalationTask(ctx, s.tasks, caseInfo.TenantID, caseID, stepID, cfg.Goal, caseInfo.CaseData, cfg.Escalation, runResult, events); err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		_ = s.traceStore.AppendEvent(ctx, &ReasoningEvent{
			TraceID:   runResult.TraceID,
			Iteration: runResult.TotalIterations,
			Sequence:  999,
			EventType: "escalation",
			Content: mustJSON(map[string]any{
				"confidence":  runResult.Confidence,
				"threshold":   cfg.Escalation.ConfidenceThreshold,
				"escalate_to": cfg.Escalation.EscalateTo,
			}),
		})
		_ = s.traceStore.UpdateTrace(ctx, &ReasoningTrace{
			ID:              runResult.TraceID,
			Status:          "escalated",
			Conclusion:      runResult.Conclusion,
			TotalIterations: runResult.TotalIterations,
			TotalToolCalls:  runResult.TotalToolCalls,
			TotalTokens:     runResult.TotalTokens,
			TotalDurationMS: runResult.DurationMS,
			CompletedAt:     &now,
		})
		return nil, engine.ErrStepAwaitingReview
	}

	patch, err := buildOutputPatch(cfg.OutputPath, runResult.Conclusion)
	if err != nil {
		return nil, err
	}
	return &engine.StepResult{
		Output:         runResult.Conclusion,
		WritesCaseData: true,
		CaseDataPatch:  patch,
		AuditEventType: "agentic.concluded",
	}, nil
}

type caseInfo struct {
	TenantID   uuid.UUID
	CaseData   json.RawMessage
	InstanceID uuid.UUID
	Model      string
}

func (s *StepExecutor) loadCaseInfo(ctx context.Context, caseID uuid.UUID) (caseInfo, error) {
	var out caseInfo
	if err := s.db.QueryRowContext(ctx, `
SELECT c.tenant_id, COALESCE(c.data, '{}'::jsonb), COALESCE(c.workflow_id, gen_random_uuid())
FROM cases c
WHERE c.id = $1
`, caseID).Scan(&out.TenantID, &out.CaseData, &out.InstanceID); err != nil {
		return caseInfo{}, fmt.Errorf("load case info: %w", err)
	}
	return out, nil
}

func (s *StepExecutor) buildManifest(ctx context.Context, tenantID, caseID uuid.UUID, cfg AgenticStepConfig) (*ToolManifest, error) {
	assembler := NewToolAssembler(s.mcpManager, s.ragSearch)
	return assembler.Assemble(ctx, tenantID, cfg.ToolPolicy, cfg.ToolNodes, func(node ToolNodeConfig, toolName string) (ToolInvoker, string, json.RawMessage, error) {
		source := strings.TrimSpace(node.Source)
		switch source {
		case "", "connector":
			safety := "read_only"
			params := json.RawMessage(`{"type":"object","properties":{"arguments":{"type":"object"}}}`)
			if s.pluginRuntime != nil {
				if rt, ok := s.pluginRuntime.(interface {
					Get(ref plugins.PluginRef) (*plugins.Plugin, error)
				}); ok {
					if plugin, err := rt.Get(plugins.PluginRef{ID: strings.TrimSpace(node.Connector)}); err == nil && plugin != nil {
						if strings.TrimSpace(plugin.ToolSafety) != "" {
							safety = strings.TrimSpace(plugin.ToolSafety)
						}
					}
				}
			}
			return invokers.NewConnectorInvoker(s.pluginRuntime, node.Connector, node.Config, cfg.Limits.Timeout), safety, params, nil
		case "mcp":
			params := json.RawMessage(`{"type":"object","properties":{}}`)
			if s.mcpManager != nil && node.MCPServerURL != "" {
				tools, err := s.mcpManager.DiscoverTools(ctx, tenantID, node.MCPServerURL, mcp.AuthConfig{Type: "none"})
				if err == nil {
					for _, tool := range tools {
						if strings.EqualFold(strings.TrimSpace(tool.Name), strings.TrimSpace(node.MCPToolName)) && len(tool.InputSchema) > 0 {
							params = tool.InputSchema
						}
					}
				}
			}
			return invokers.NewMCPInvoker(s.mcpManager, tenantID, node.MCPServerURL, node.MCPToolName, mcp.AuthConfig{Type: "none"}, 1), "read_only", params, nil
		case "rag":
			kbID, err := uuid.Parse(strings.TrimSpace(node.KnowledgeBase))
			if err != nil {
				return nil, "", nil, fmt.Errorf("invalid rag knowledge_base")
			}
			return invokers.NewRAGInvoker(s.ragSearch, tenantID, kbID), "read_only", json.RawMessage(`{
  "type":"object",
  "properties":{"query":{"type":"string"},"top_k":{"type":"integer","minimum":1}},
  "required":["query"]
}`), nil
		case "case_data":
			readOnly := cfg.ToolPolicy.ToolMode.Normalize() == ToolModeReadOnly
			return invokers.NewCaseDataInvoker(&stepCaseStore{db: s.db, tenantID: tenantID, caseID: caseID}, readOnly), map[bool]string{true: "read_only", false: "idempotent_write"}[readOnly], json.RawMessage(`{
  "type":"object",
  "properties":{"path":{"type":"string"},"value":{}},
  "required":["path"]
}`), nil
		default:
			return nil, "", nil, fmt.Errorf("unsupported tool source: %s", source)
		}
	})
}

type stepCaseStore struct {
	db       *sql.DB
	tenantID uuid.UUID
	caseID   uuid.UUID
}

func (s *stepCaseStore) GetCaseData(ctx context.Context) (map[string]any, error) {
	var raw json.RawMessage
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(data, '{}'::jsonb) FROM cases WHERE tenant_id = $1 AND id = $2`, s.tenantID, s.caseID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load case data: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode case data: %w", err)
	}
	return out, nil
}

func (s *stepCaseStore) MergeCaseData(ctx context.Context, patch map[string]any) error {
	raw, _ := json.Marshal(patch)
	_, err := s.db.ExecContext(ctx, `
UPDATE cases
SET data = COALESCE(data, '{}'::jsonb) || $3::jsonb,
    version = version + 1,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2
`, s.tenantID, s.caseID, string(raw))
	if err != nil {
		return fmt.Errorf("merge case data: %w", err)
	}
	return nil
}

func buildOutputPatch(path string, conclusion json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(conclusion, &value); err != nil {
		return nil, fmt.Errorf("decode conclusion output: %w", err)
	}
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "case.")
	path = strings.TrimPrefix(path, "data.")
	if path == "" {
		raw, _ := json.Marshal(value)
		return raw, nil
	}
	parts := strings.Split(path, ".")
	root := map[string]any{}
	cur := root
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if i == len(parts)-1 {
			cur[p] = value
			break
		}
		next := map[string]any{}
		cur[p] = next
		cur = next
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal output patch: %w", err)
	}
	return raw, nil
}
