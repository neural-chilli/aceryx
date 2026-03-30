package agents

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

type AgentExecutor struct {
	db              *sql.DB
	tasks           TaskCreator
	prompts         *PromptTemplateService
	llm             *LLMClient
	defaultModel    string
	contextTimeout  time.Duration
	sourceTimeout   time.Duration
	contextMaxBytes int
	llmTimeout      time.Duration
}

func NewAgentExecutor(cfg ExecutorConfig) *AgentExecutor {
	ctxTimeout := cfg.ContextTimeout
	if ctxTimeout <= 0 {
		ctxTimeout = defaultContextTimeout
	}
	srcTimeout := cfg.SourceTimeout
	if srcTimeout <= 0 {
		srcTimeout = defaultSourceTimeout
	}
	ctxMaxBytes := cfg.ContextMaxBytes
	if ctxMaxBytes <= 0 {
		ctxMaxBytes = defaultContextMaxBytes
	}
	llmTimeout := cfg.LLMTimeout
	if llmTimeout <= 0 {
		llmTimeout = defaultLLMTimeout
	}
	llm := cfg.LLMClient
	if llm == nil {
		llm = NewLLMClientFromEnv(llmTimeout)
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" && llm != nil {
		model = llm.model
	}
	return &AgentExecutor{
		db:              cfg.DB,
		tasks:           cfg.TaskCreator,
		prompts:         NewPromptTemplateService(cfg.DB),
		llm:             llm,
		defaultModel:    model,
		contextTimeout:  ctxTimeout,
		sourceTimeout:   srcTimeout,
		contextMaxBytes: ctxMaxBytes,
		llmTimeout:      llmTimeout,
	}
}

func (a *AgentExecutor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, raw json.RawMessage) (*engine.StepResult, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("agent executor not configured")
	}

	cfg := StepConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse agent step config: %w", err)
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultValidationMaxAttempt
	}

	tenantID, caseNumber, caseTypeName, caseData, stepResults, err := a.loadCaseAndSteps(ctx, caseID)
	if err != nil {
		return nil, err
	}

	ctxStage, cancelCtx := context.WithTimeout(ctx, a.contextTimeout)
	defer cancelCtx()
	assembled, err := a.ResolveContext(ctxStage, tenantID, caseID, caseData, stepResults, cfg.Context)
	if err != nil {
		return nil, fmt.Errorf("assemble agent context: %w", err)
	}

	tpl, err := a.prompts.resolveTemplate(ctx, tenantID, cfg.PromptTemplate, cfg.PromptVersion)
	if err != nil {
		return nil, err
	}

	model := a.defaultModel
	if strings.TrimSpace(cfg.Model) != "" && cfg.Model != "default" {
		model = strings.TrimSpace(cfg.Model)
	}
	if model == "" && a.llm != nil {
		model = a.llm.model
	}
	if model == "" {
		return nil, fmt.Errorf("llm model not configured")
	}

	outputSchemaAny := map[string]any{}
	for k, v := range cfg.OutputSchema {
		outputSchemaAny[k] = v
	}
	promptData := map[string]any{
		"case":          assembled.Case,
		"steps":         assembled.Steps,
		"knowledge":     assembled.Knowledge,
		"vault":         assembled.Vault,
		"output_schema": outputSchemaAny,
		"case_metadata": map[string]any{
			"case_number": caseNumber,
			"case_type":   caseTypeName,
		},
		"now": time.Now().UTC().Format(time.RFC3339),
	}

	renderedPrompt, err := renderPromptTemplate(tpl.Template, promptData)
	if err != nil {
		return nil, err
	}
	promptHash := sha256.Sum256([]byte(renderedPrompt))

	resultObj, usage, latencyMs, err := a.invokeWithValidationRetry(ctx, model, renderedPrompt, cfg)
	if err != nil {
		return nil, err
	}

	confidence, _ := asFloat(resultObj["confidence"])
	if confidence < cfg.ConfidenceThreshold && strings.EqualFold(cfg.OnLowConfidence, "escalate_to_human") {
		if err := a.createHumanReviewTask(ctx, caseID, stepID, cfg, resultObj, confidence); err != nil {
			return nil, err
		}
		return nil, engine.ErrStepAwaitingReview
	}

	resultPayload, err := json.Marshal(resultObj)
	if err != nil {
		return nil, fmt.Errorf("marshal agent output: %w", err)
	}

	event := map[string]any{
		"type":                 "llm_call",
		"model":                model,
		"prompt_template":      fmt.Sprintf("%s_v%d", tpl.Name, tpl.Version),
		"tokens":               map[string]any{"input": usage.InputTokens, "output": usage.OutputTokens},
		"latency_ms":           latencyMs,
		"confidence":           confidence,
		"context_snapshot":     assembled.Meta,
		"output":               resultObj,
		"rendered_prompt":      renderedPrompt,
		"rendered_prompt_hash": "sha256:" + hex.EncodeToString(promptHash[:]),
	}
	eventJSON, _ := json.Marshal(event)

	stepResult := &engine.StepResult{
		Output:         resultPayload,
		ExecutionEvent: eventJSON,
		AuditEventType: "agent_step_completed",
	}
	if cfg.WritesCaseData {
		stepResult.WritesCaseData = true
		if cfg.CaseDataField != "" {
			stepResult.CaseDataPatch, _ = json.Marshal(map[string]any{cfg.CaseDataField: resultObj})
		} else {
			stepResult.CaseDataPatch, _ = json.Marshal(resultObj)
		}
	}
	return stepResult, nil
}

func (a *AgentExecutor) ResolveContext(ctx context.Context, tenantID, caseID uuid.UUID, caseData map[string]any, stepResults map[string]any, sources []ContextSource) (*AssembledContext, error) {
	ctxData := &AssembledContext{Case: map[string]any{}, Steps: map[string]any{}, Knowledge: []KnowledgeResult{}, Vault: []VaultDocResult{}, Meta: ContextSnapshotMeta{Sources: []SourceSnapshot{}}}
	if len(sources) == 0 {
		ctxData.Case = caseData
		ctxData.Meta.Sources = append(ctxData.Meta.Sources, SourceSnapshot{Source: "case", Bytes: approxJSONBytes(caseData)})
		ctxData.Meta.TotalSizeBytes = approxJSONBytes(caseData)
		return ctxData, nil
	}

	largestSource := ""
	largestBytes := 0
	totalBytes := 0

	for _, src := range sources {
		sourceCtx, cancel := context.WithTimeout(ctx, a.sourceTimeout)
		var (
			bytes int
			refs  []string
		)
		srcName := strings.TrimSpace(src.Source)
		switch srcName {
		case "case":
			resolved := caseData
			if len(src.Fields) > 0 {
				resolved = pickMapFields(caseData, src.Fields)
			}
			ctxData.Case = mergeMap(ctxData.Case, resolved)
			bytes = approxJSONBytes(resolved)
		case "steps":
			resolved := pickStepFields(stepResults, src.Fields)
			ctxData.Steps = mergeMap(ctxData.Steps, resolved)
			bytes = approxJSONBytes(resolved)
		case "knowledge":
			query := connectors.ResolveTemplateString(src.Query, map[string]any{"case": caseData, "steps": stepResults, "now": time.Now().UTC().Format(time.RFC3339)})
			topK := src.TopK
			if topK <= 0 {
				topK = defaultKnowledgeTopK
			}
			results, err := a.queryKnowledge(sourceCtx, tenantID, query, src.Collection, topK)
			cancel()
			if err != nil {
				return nil, err
			}
			ctxData.Knowledge = append(ctxData.Knowledge, results...)
			for _, item := range results {
				refs = append(refs, item.DocumentID.String())
			}
			bytes = approxJSONBytes(results)
			ctxData.Meta.Sources = append(ctxData.Meta.Sources, SourceSnapshot{Source: "knowledge", Bytes: bytes, Refs: refs})
			totalBytes += bytes
			if bytes > largestBytes {
				largestBytes = bytes
				largestSource = "knowledge"
			}
			if totalBytes > a.contextMaxBytes {
				return nil, fmt.Errorf("assembled context exceeds %d bytes; largest source: %s", a.contextMaxBytes, largestSource)
			}
			continue
		case "vault":
			results, err := a.queryVaultDocuments(sourceCtx, tenantID, caseID, src.DocumentTypes)
			cancel()
			if err != nil {
				return nil, err
			}
			ctxData.Vault = append(ctxData.Vault, results...)
			for _, item := range results {
				refs = append(refs, item.DocumentID.String())
			}
			bytes = approxJSONBytes(results)
			ctxData.Meta.Sources = append(ctxData.Meta.Sources, SourceSnapshot{Source: "vault", Bytes: bytes, Refs: refs})
			totalBytes += bytes
			if bytes > largestBytes {
				largestBytes = bytes
				largestSource = "vault"
			}
			if totalBytes > a.contextMaxBytes {
				return nil, fmt.Errorf("assembled context exceeds %d bytes; largest source: %s", a.contextMaxBytes, largestSource)
			}
			continue
		default:
			cancel()
			return nil, fmt.Errorf("unsupported context source %q", src.Source)
		}
		cancel()

		totalBytes += bytes
		ctxData.Meta.Sources = append(ctxData.Meta.Sources, SourceSnapshot{Source: srcName, Bytes: bytes, Refs: refs})
		if bytes > largestBytes {
			largestBytes = bytes
			largestSource = srcName
		}
		if totalBytes > a.contextMaxBytes {
			return nil, fmt.Errorf("assembled context exceeds %d bytes; largest source: %s", a.contextMaxBytes, largestSource)
		}
	}
	ctxData.Meta.TotalSizeBytes = totalBytes
	return ctxData, nil
}

func (a *AgentExecutor) queryKnowledge(ctx context.Context, tenantID uuid.UUID, query string, collection string, topK int) ([]KnowledgeResult, error) {
	if strings.TrimSpace(query) == "" {
		return []KnowledgeResult{}, nil
	}
	embedding, err := a.llm.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed knowledge query: %w", err)
	}
	vec := vectorLiteral(embedding)

	filterCollection := strings.TrimSpace(collection)
	querySQL := `
SELECT id, filename, COALESCE(extracted_text, ''), 1 - (embedding <=> $2::vector) AS similarity
FROM vault_documents
WHERE tenant_id = $1
  AND deleted_at IS NULL
  AND embedding IS NOT NULL
`
	args := []any{tenantID, vec}
	if filterCollection != "" {
		querySQL += ` AND COALESCE(metadata->>'collection','') = $3`
		args = append(args, filterCollection)
		querySQL += ` ORDER BY embedding <=> $2::vector LIMIT $4`
		args = append(args, topK)
	} else {
		querySQL += ` ORDER BY embedding <=> $2::vector LIMIT $3`
		args = append(args, topK)
	}

	rows, err := a.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query knowledge documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]KnowledgeResult, 0)
	for rows.Next() {
		var item KnowledgeResult
		if err := rows.Scan(&item.DocumentID, &item.Filename, &item.Text, &item.Similarity); err != nil {
			return nil, fmt.Errorf("scan knowledge result: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge results: %w", err)
	}
	return results, nil
}

func (a *AgentExecutor) queryVaultDocuments(ctx context.Context, tenantID, caseID uuid.UUID, documentTypes []string) ([]VaultDocResult, error) {
	querySQL := `
SELECT id, filename, mime_type, COALESCE(extracted_text,''), COALESCE(metadata, '{}'::jsonb)
FROM vault_documents
WHERE tenant_id = $1
  AND case_id = $2
  AND deleted_at IS NULL
`
	args := []any{tenantID, caseID}
	if len(documentTypes) > 0 {
		querySQL += ` AND COALESCE(metadata->>'document_type','') = ANY($3)`
		args = append(args, toPGTextArray(documentTypes))
	}
	querySQL += ` ORDER BY uploaded_at DESC LIMIT 100`

	rows, err := a.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query vault context documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]VaultDocResult, 0)
	for rows.Next() {
		var item VaultDocResult
		if err := rows.Scan(&item.DocumentID, &item.Filename, &item.MimeType, &item.ExtractedText, &item.Metadata); err != nil {
			return nil, fmt.Errorf("scan vault context document: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vault context documents: %w", err)
	}
	return out, nil
}

func (a *AgentExecutor) invokeWithValidationRetry(ctx context.Context, model string, renderedPrompt string, cfg StepConfig) (map[string]any, Usage, int, error) {
	start := time.Now()
	messages := []Message{
		{Role: "system", Content: "You are an assistant that must return strict JSON matching the requested schema."},
		{Role: "user", Content: renderedPrompt},
	}
	responseFormat := &ResponseFormat{Type: "json_schema", JSONSchema: map[string]any{"name": "agent_output", "schema": cfg.OutputSchema}}

	var lastErr error
	var usage Usage
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		client := *a.llm
		client.model = model
		llmCtx, cancel := context.WithTimeout(ctx, a.llmTimeout)
		resp, err := client.ChatCompletion(llmCtx, messages, responseFormat)
		cancel()
		if err != nil {
			return nil, usage, 0, err
		}
		usage = resp.Usage

		parsed := map[string]any{}
		if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
			lastErr = fmt.Errorf("invalid json output: %w", err)
		} else if err := validateOutputAgainstSchema(parsed, cfg.OutputSchema); err != nil {
			lastErr = err
		} else {
			if _, ok := asFloat(parsed["confidence"]); !ok {
				lastErr = fmt.Errorf("missing confidence field")
			} else {
				return parsed, usage, int(time.Since(start).Milliseconds()), nil
			}
		}

		corrective := fmt.Sprintf("Your previous response did not match the required schema. Error: %s. Please respond again with valid JSON matching: %s", lastErr.Error(), mustJSON(cfg.OutputSchema))
		messages = []Message{{Role: "system", Content: "You must return only valid JSON."}, {Role: "user", Content: corrective}}
	}
	if lastErr == nil {
		lastErr = errors.New("validation failed")
	}
	return nil, usage, int(time.Since(start).Milliseconds()), lastErr
}

func (a *AgentExecutor) createHumanReviewTask(ctx context.Context, caseID uuid.UUID, stepID string, cfg StepConfig, output map[string]any, confidence float64) error {
	if a.tasks == nil {
		return fmt.Errorf("task service not configured for low-confidence escalation")
	}
	fields := make([]tasks.FormField, 0, len(cfg.OutputSchema))
	keys := make([]string, 0, len(cfg.OutputSchema))
	for key := range cfg.OutputSchema {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		def := cfg.OutputSchema[key]
		fieldType := def.Type
		if fieldType == "" {
			fieldType = "text"
		}
		if fieldType == "number" {
			fieldType = "number"
		} else {
			fieldType = "text"
		}
		fields = append(fields, tasks.FormField{ID: key, Type: fieldType, Required: true, Bind: "decision." + key})
	}

	cfgTask := tasks.AssignmentConfig{
		AssignToRole: cfg.AssignToRole,
		AssignToUser: cfg.AssignToUser,
		SLAHours:     cfg.SLAHours,
		Escalation:   cfg.Escalation,
		Form:         "agent_review",
		FormSchema:   tasks.FormSchema{Fields: fields},
		Outcomes:     []string{"accept", "modify", "override"},
		Metadata: map[string]any{
			"agent_review": map[string]any{
				"original_output": output,
				"confidence":      confidence,
				"reasoning":       output["reasoning"],
				"flags":           output["flags"],
			},
		},
	}
	if cfgTask.AssignToRole == "" && cfgTask.AssignToUser == "" {
		cfgTask.AssignToRole = "case_worker"
	}
	return a.tasks.CreateTaskFromActivation(ctx, caseID, stepID, cfgTask)
}

func (a *AgentExecutor) loadCaseAndSteps(ctx context.Context, caseID uuid.UUID) (uuid.UUID, string, string, map[string]any, map[string]any, error) {
	var (
		tenantID   uuid.UUID
		caseNumber string
		caseType   string
		caseDataB  []byte
	)
	err := a.db.QueryRowContext(ctx, `
SELECT c.tenant_id, c.case_number, ct.name, c.data
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.id = $1
`, caseID).Scan(&tenantID, &caseNumber, &caseType, &caseDataB)
	if err != nil {
		return uuid.Nil, "", "", nil, nil, fmt.Errorf("load case context: %w", err)
	}
	caseData := map[string]any{}
	_ = json.Unmarshal(caseDataB, &caseData)

	rows, err := a.db.QueryContext(ctx, `
SELECT step_id, COALESCE(result, '{}'::jsonb)
FROM case_steps
WHERE case_id = $1
`, caseID)
	if err != nil {
		return uuid.Nil, "", "", nil, nil, fmt.Errorf("load step context: %w", err)
	}
	defer func() { _ = rows.Close() }()

	steps := map[string]any{}
	for rows.Next() {
		var sid string
		var raw []byte
		if err := rows.Scan(&sid, &raw); err != nil {
			return uuid.Nil, "", "", nil, nil, fmt.Errorf("scan step context: %w", err)
		}
		decoded := map[string]any{}
		_ = json.Unmarshal(raw, &decoded)
		steps[sid] = map[string]any{"result": decoded}
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, "", "", nil, nil, fmt.Errorf("iterate step context: %w", err)
	}
	return tenantID, caseNumber, caseType, caseData, steps, nil
}

func pickMapFields(src map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return cloneMap(src)
	}
	out := map[string]any{}
	for _, field := range fields {
		if v, ok := lookupDotPath(src, field); ok {
			setDotPath(out, field, v)
		}
	}
	return out
}

func pickStepFields(stepResults map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return cloneMap(stepResults)
	}
	out := map[string]any{}
	for _, field := range fields {
		parts := strings.Split(field, ".")
		if len(parts) == 0 {
			continue
		}
		stepID := parts[0]
		rest := strings.Join(parts[1:], ".")
		stepDataRaw, ok := stepResults[stepID]
		if !ok {
			continue
		}
		stepData, _ := stepDataRaw.(map[string]any)
		if rest == "" {
			out[stepID] = stepData
			continue
		}
		if v, ok := lookupDotPath(stepData, rest); ok {
			stepOut, _ := out[stepID].(map[string]any)
			if stepOut == nil {
				stepOut = map[string]any{}
				out[stepID] = stepOut
			}
			setDotPath(stepOut, rest, v)
		}
	}
	return out
}

func lookupDotPath(root map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = root
	for _, part := range parts {
		nextMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := nextMap[part]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func setDotPath(root map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := root
	for i, part := range parts {
		if i == len(parts)-1 {
			cur[part] = value
			return
		}
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
}

func mergeMap(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			existing, _ := dst[k].(map[string]any)
			dst[k] = mergeMap(existing, vMap)
			continue
		}
		dst[k] = v
	}
	return dst
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if mv, ok := v.(map[string]any); ok {
			out[k] = cloneMap(mv)
		} else {
			out[k] = v
		}
	}
	return out
}

func approxJSONBytes(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}

func vectorLiteral(v []float32) string {
	parts := make([]string, 0, len(v))
	for _, n := range v {
		parts = append(parts, strconv.FormatFloat(float64(n), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

type pgTextArray []string

func toPGTextArray(items []string) pgTextArray {
	return pgTextArray(items)
}

func (a pgTextArray) Value() (driver.Value, error) {
	quoted := make([]string, len(a))
	for i, v := range a {
		quoted[i] = `"` + strings.ReplaceAll(v, `"`, `\\"`) + `"`
	}
	return "{" + strings.Join(quoted, ",") + "}", nil
}
