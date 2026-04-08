package assistant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/agents"
	"gopkg.in/yaml.v3"
)

type API struct {
	db               *sql.DB
	llm              *agents.LLMClient
	logPrompts       bool
	logPromptMaxSize int
}

func NewAPI(db *sql.DB, llm *agents.LLMClient) *API {
	return &API{
		db:               db,
		llm:              llm,
		logPrompts:       boolEnv("ACERYX_ASSISTANT_LOG_PROMPTS"),
		logPromptMaxSize: intEnvOrDefault("ACERYX_ASSISTANT_LOG_PROMPT_MAX_CHARS", 24000),
	}
}

func (a *API) CreateSession(ctx context.Context, tenantID, userID uuid.UUID, pageContext string) (*Session, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("assistant api not configured")
	}
	pageContext = strings.TrimSpace(pageContext)
	if pageContext == "" {
		pageContext = "builder"
	}
	var out Session
	err := a.db.QueryRowContext(ctx, `
INSERT INTO ai_assistant_sessions (tenant_id, user_id, page_context)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, user_id, page_context, created_at, updated_at
`, tenantID, userID, pageContext).Scan(
		&out.ID, &out.TenantID, &out.UserID, &out.PageContext, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create assistant session: %w", err)
	}
	return &out, nil
}

func (a *API) GetSession(ctx context.Context, tenantID, sessionID uuid.UUID) (*SessionWithMessages, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("assistant api not configured")
	}
	var out SessionWithMessages
	err := a.db.QueryRowContext(ctx, `
SELECT id, tenant_id, user_id, page_context, created_at, updated_at
FROM ai_assistant_sessions
WHERE id = $1 AND tenant_id = $2
`, sessionID, tenantID).Scan(
		&out.Session.ID, &out.Session.TenantID, &out.Session.UserID, &out.Session.PageContext, &out.Session.CreatedAt, &out.Session.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	rows, err := a.db.QueryContext(ctx, `
SELECT id, session_id, role, content, COALESCE(mode, ''), COALESCE(yaml_before, ''), COALESCE(yaml_after, ''),
       COALESCE(diff, ''), applied, COALESCE(model_used, ''), COALESCE(tokens_used, 0), created_at
FROM ai_assistant_messages
WHERE session_id = $1
ORDER BY created_at ASC
`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list assistant messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var m Message
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Mode, &m.YAMLBefore, &m.YAMLAfter,
			&m.Diff, &m.Applied, &m.ModelUsed, &m.TokensUsed, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan assistant message: %w", err)
		}
		out.Messages = append(out.Messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assistant messages: %w", err)
	}
	return &out, nil
}

func (a *API) DeleteSession(ctx context.Context, tenantID, sessionID uuid.UUID) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("assistant api not configured")
	}
	res, err := a.db.ExecContext(ctx, `
DELETE FROM ai_assistant_sessions
WHERE id = $1 AND tenant_id = $2
`, sessionID, tenantID)
	if err != nil {
		return fmt.Errorf("delete assistant session: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *API) Message(ctx context.Context, tenantID, userID uuid.UUID, req MessageRequest) (*MessageResponse, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("assistant api not configured")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	mode := normalizeMode(req.Mode)
	if err := assertBuilderContractVersion(mode, req.PageContext, req.PromptPack); err != nil {
		return nil, err
	}
	sessionID, err := a.resolveSession(ctx, tenantID, userID, req)
	if err != nil {
		return nil, err
	}

	workflowYAMLBefore := ""
	if req.WorkflowID != nil {
		workflowYAMLBefore, err = a.getWorkflowYAML(ctx, tenantID, *req.WorkflowID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("workflow not found")
			}
			return nil, err
		}
	}

	assistantContent, modelUsed, tokensUsed, err := a.generateAssistantContent(ctx, mode, content, workflowYAMLBefore, req.PageContext, req.PromptPack)
	if err != nil {
		return nil, err
	}

	yamlAfter := ""
	diff := ""
	if mode == ModeDescribe || mode == ModeRefactor {
		candidate := extractYAML(assistantContent)
		normalized, err := normalizeToBuilderASTYAML(candidate)
		if err == nil {
			yamlAfter = normalized
			diff = computeUnifiedDiff(workflowYAMLBefore, yamlAfter)
		}
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin assistant message tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
UPDATE ai_assistant_sessions
SET updated_at = now()
WHERE id = $1 AND tenant_id = $2
`, sessionID, tenantID); err != nil {
		return nil, fmt.Errorf("touch assistant session: %w", err)
	}

	assistantMessageID := uuid.New()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO ai_assistant_messages (
    id, session_id, role, content, mode, yaml_before, yaml_after, diff, applied, model_used, tokens_used
)
VALUES ($1, $2, 'assistant', $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), false, NULLIF($8, ''), $9)
`, assistantMessageID, sessionID, assistantContent, mode, workflowYAMLBefore, yamlAfter, diff, modelUsed, tokensUsed); err != nil {
		return nil, fmt.Errorf("insert assistant message: %w", err)
	}

	if diff != "" && req.WorkflowID != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ai_assistant_diffs (tenant_id, workflow_id, message_id, user_id, prompt, diff, applied)
VALUES ($1, $2, $3, $4, $5, $6, false)
`, tenantID, *req.WorkflowID, assistantMessageID, userID, content, diff); err != nil {
			return nil, fmt.Errorf("insert assistant diff: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit assistant message tx: %w", err)
	}

	return &MessageResponse{
		SessionID: sessionID,
		MessageID: assistantMessageID,
		Content:   assistantContent,
		YAMLAfter: yamlAfter,
		Diff:      diff,
		Applied:   false,
	}, nil
}

func (a *API) ListDiffs(ctx context.Context, tenantID, workflowID uuid.UUID) ([]DiffRecord, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("assistant api not configured")
	}
	rows, err := a.db.QueryContext(ctx, `
SELECT id, tenant_id, workflow_id, message_id, user_id, prompt, diff, applied, applied_at, created_at
FROM ai_assistant_diffs
WHERE tenant_id = $1 AND workflow_id = $2
ORDER BY created_at DESC
`, tenantID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list assistant diffs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]DiffRecord, 0)
	for rows.Next() {
		var item DiffRecord
		if err := rows.Scan(
			&item.ID, &item.TenantID, &item.WorkflowID, &item.MessageID, &item.UserID,
			&item.Prompt, &item.Diff, &item.Applied, &item.AppliedAt, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan assistant diff: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assistant diffs: %w", err)
	}
	return out, nil
}

func (a *API) ApplyDiff(ctx context.Context, tenantID uuid.UUID, diffID uuid.UUID) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("assistant api not configured")
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin apply diff tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var record DiffRecord
	err = tx.QueryRowContext(ctx, `
SELECT id, tenant_id, workflow_id, message_id, user_id, prompt, diff, applied, applied_at, created_at
FROM ai_assistant_diffs
WHERE id = $1 AND tenant_id = $2
`, diffID, tenantID).Scan(
		&record.ID, &record.TenantID, &record.WorkflowID, &record.MessageID, &record.UserID,
		&record.Prompt, &record.Diff, &record.Applied, &record.AppliedAt, &record.CreatedAt,
	)
	if err != nil {
		return err
	}

	var yamlAfter string
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(yaml_after, '')
FROM ai_assistant_messages
WHERE id = $1
`, record.MessageID).Scan(&yamlAfter); err != nil {
		return fmt.Errorf("load assistant message yaml: %w", err)
	}

	if strings.TrimSpace(yamlAfter) != "" {
		var currentMax int
		err = tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(wv.version), 0)
FROM workflow_versions wv
JOIN workflows w ON w.id = wv.workflow_id
WHERE w.id = $1 AND w.tenant_id = $2
`, record.WorkflowID, tenantID).Scan(&currentMax)
		if err != nil {
			return fmt.Errorf("load workflow version: %w", err)
		}

		var draftID uuid.UUID
		err = tx.QueryRowContext(ctx, `
SELECT wv.id
FROM workflow_versions wv
JOIN workflows w ON w.id = wv.workflow_id
WHERE w.id = $1 AND w.tenant_id = $2 AND wv.status = 'draft'
ORDER BY wv.version DESC
LIMIT 1
`, record.WorkflowID, tenantID).Scan(&draftID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load draft workflow version: %w", err)
		}

		if errors.Is(err, sql.ErrNoRows) {
			newVersion := currentMax + 1
			if _, err := tx.ExecContext(ctx, `
INSERT INTO workflow_versions (workflow_id, version, status, ast, yaml_source, created_by)
VALUES ($1, $2, 'draft', '{}'::jsonb, $3, $4)
`, record.WorkflowID, newVersion, yamlAfter, record.UserID); err != nil {
				return fmt.Errorf("create draft workflow version: %w", err)
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
UPDATE workflow_versions
SET yaml_source = $2
WHERE id = $1
`, draftID, yamlAfter); err != nil {
				return fmt.Errorf("update draft workflow version: %w", err)
			}
		}
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
UPDATE ai_assistant_diffs
SET applied = true, applied_at = $2
WHERE id = $1 AND tenant_id = $3
`, diffID, now, tenantID); err != nil {
		return fmt.Errorf("update diff apply state: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE ai_assistant_messages
SET applied = true
WHERE id = $1
`, record.MessageID); err != nil {
		return fmt.Errorf("update message apply state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit apply diff tx: %w", err)
	}
	return nil
}

func (a *API) RejectDiff(ctx context.Context, tenantID uuid.UUID, diffID uuid.UUID) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("assistant api not configured")
	}
	res, err := a.db.ExecContext(ctx, `
UPDATE ai_assistant_diffs
SET applied = false, applied_at = NULL
WHERE id = $1 AND tenant_id = $2
`, diffID, tenantID)
	if err != nil {
		return fmt.Errorf("reject assistant diff: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *API) resolveSession(ctx context.Context, tenantID, userID uuid.UUID, req MessageRequest) (uuid.UUID, error) {
	if req.SessionID != nil {
		var exists bool
		if err := a.db.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM ai_assistant_sessions
    WHERE id = $1 AND tenant_id = $2
)
`, *req.SessionID, tenantID).Scan(&exists); err != nil {
			return uuid.Nil, fmt.Errorf("check assistant session: %w", err)
		}
		if !exists {
			return uuid.Nil, sql.ErrNoRows
		}
		return *req.SessionID, nil
	}

	session, err := a.CreateSession(ctx, tenantID, userID, req.PageContext)
	if err != nil {
		return uuid.Nil, err
	}
	return session.ID, nil
}

func (a *API) getWorkflowYAML(ctx context.Context, tenantID, workflowID uuid.UUID) (string, error) {
	var yamlSource string
	err := a.db.QueryRowContext(ctx, `
SELECT COALESCE(wv.yaml_source, '')
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.id = $1
  AND w.tenant_id = $2
  AND wv.status IN ('draft', 'published')
ORDER BY CASE wv.status WHEN 'draft' THEN 0 ELSE 1 END, wv.version DESC
LIMIT 1
`, workflowID, tenantID).Scan(&yamlSource)
	if err != nil {
		return "", err
	}
	return yamlSource, nil
}

func (a *API) generateAssistantContent(
	ctx context.Context,
	mode, userPrompt, yamlBefore, pageContext string,
	promptPack *PromptPackInput,
) (string, string, int, error) {
	if a.llm == nil {
		return fallbackResponse(mode, userPrompt, yamlBefore), "", 0, nil
	}

	systemPrompt := assistantSystemPrompt()
	userPromptPayload := composeAssistantUserPrompt(mode, userPrompt, yamlBefore, pageContext, promptPack)

	if a.logPrompts {
		slog.InfoContext(ctx, "assistant llm prompt",
			"mode", mode,
			"page_context", strings.TrimSpace(pageContext),
			"system_prompt", trimForLog(systemPrompt, a.logPromptMaxSize),
			"user_prompt", trimForLog(userPromptPayload, a.logPromptMaxSize),
			"system_prompt_chars", len(systemPrompt),
			"user_prompt_chars", len(userPromptPayload),
		)
	}

	resp, err := a.llm.ChatCompletion(ctx, []agents.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPromptPayload},
	}, nil)
	if err != nil {
		return fallbackResponse(mode, userPrompt, yamlBefore), "", 0, nil
	}
	return strings.TrimSpace(resp.Content), strings.TrimSpace(resp.Model), resp.Usage.InputTokens + resp.Usage.OutputTokens, nil
}

func boolEnv(key string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func intEnvOrDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func trimForLog(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "\n...[truncated]"
}

func fallbackResponse(mode, userPrompt, yamlBefore string) string {
	switch mode {
	case ModeExplain:
		if strings.TrimSpace(yamlBefore) == "" {
			return "No workflow is currently selected. Select a workflow and ask again for an explanation."
		}
		return "The current workflow has been loaded. AI provider is not configured, so detailed explanation is unavailable in this environment."
	case ModeTestGenerate:
		return "Feature: Workflow\n\n  Scenario: Placeholder\n    Given the workflow is configured\n    When the user triggers it\n    Then the expected outcome is produced"
	default:
		if strings.TrimSpace(yamlBefore) != "" {
			return yamlBefore
		}
		return "steps: []"
	}
}

func normalizeToBuilderASTYAML(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty yaml")
	}
	var decoded map[string]any
	if err := yaml.Unmarshal([]byte(raw), &decoded); err != nil {
		return "", fmt.Errorf("invalid yaml: %w", err)
	}

	if steps, ok := decoded["steps"].([]any); ok {
		decoded["steps"] = steps
		if err := normalizeBuilderAST(decoded); err != nil {
			return "", err
		}
		out, err := yaml.Marshal(decoded)
		if err != nil {
			return "", fmt.Errorf("marshal yaml: %w", err)
		}
		return string(out), nil
	}

	if wf, ok := decoded["workflow"].(map[string]any); ok {
		if steps, ok := wf["steps"].([]any); ok {
			ast := map[string]any{
				"steps": steps,
			}
			if name, ok := wf["name"]; ok {
				ast["name"] = name
			}
			if caseTypeID, ok := wf["case_type_id"]; ok {
				ast["case_type_id"] = caseTypeID
			}
			if err := normalizeBuilderAST(ast); err != nil {
				return "", err
			}
			out, err := yaml.Marshal(ast)
			if err != nil {
				return "", fmt.Errorf("marshal yaml: %w", err)
			}
			return string(out), nil
		}
	}

	return "", fmt.Errorf("builder AST yaml must include top-level steps array")
}

func normalizeBuilderAST(ast map[string]any) error {
	steps, ok := ast["steps"].([]any)
	if !ok {
		return nil
	}
	for _, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if err := normalizeBuilderStep(step); err != nil {
			return err
		}
	}
	return nil
}

func normalizeBuilderStep(step map[string]any) error {
	stepType, err := normalizeBuilderStepType(asTrimmedString(step["type"]))
	if err != nil {
		return err
	}
	if stepType != "" {
		step["type"] = stepType
	}

	cfg, _ := step["config"].(map[string]any)
	if cfg == nil {
		return nil
	}

	switch stepType {
	case "agent":
		normalizeAgentToAIComponentStep(cfg, step)
	case "integration":
		if asTrimmedString(cfg["connector"]) == "" && asTrimmedString(cfg["integration"]) != "" {
			cfg["connector"] = asTrimmedString(cfg["integration"])
		}
		normalizeIntegrationConfig(cfg)
	case "ai_component":
		normalizeAIComponentConfig(cfg)
	case "human_task":
		normalizeHumanTaskForm(cfg, step)
	case "extraction":
		normalizeExtractionConfig(cfg)
	case "rule":
		normalizeRuleConfig(cfg, step)
	}
	return nil
}

func normalizeAgentToAIComponentStep(cfg map[string]any, step map[string]any) bool {
	componentID := asTrimmedString(cfg["component"])
	if componentID == "" {
		componentID = asTrimmedString(cfg["ai_component_id"])
	}
	if componentID == "" {
		return false
	}
	step["type"] = "ai_component"
	cfg["component"] = componentID
	delete(cfg, "ai_component_id")
	normalizeAIComponentConfig(cfg)
	return true
}

func normalizeAIComponentConfig(cfg map[string]any) {
	if asTrimmedString(cfg["output_path"]) == "" {
		componentID := asTrimmedString(cfg["component"])
		if componentID != "" {
			cfg["output_path"] = "case.data.ai." + normalizePathToken(componentID)
		}
	}
	if _, ok := cfg["input_paths"]; !ok {
		if mapped, ok := cfg["input_mapping"].(map[string]any); ok && len(mapped) > 0 {
			cfg["input_paths"] = normalizeStringMap(mapped)
		}
	}
	delete(cfg, "input_mapping")
	if _, ok := cfg["config_values"]; !ok {
		if mapped, ok := cfg["config"].(map[string]any); ok && len(mapped) > 0 {
			cfg["config_values"] = normalizeStringMap(mapped)
		}
	}
	delete(cfg, "config")
}

func normalizeStringMap(raw map[string]any) map[string]string {
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		trimmedValue := asTrimmedString(value)
		if trimmedValue == "" {
			continue
		}
		out[trimmedKey] = trimmedValue
	}
	return out
}

func normalizePathToken(raw string) string {
	token := strings.ToLower(strings.TrimSpace(raw))
	token = strings.ReplaceAll(token, "-", "_")
	token = strings.ReplaceAll(token, ".", "_")
	token = strings.ReplaceAll(token, " ", "_")
	if token == "" {
		return "result"
	}
	var b strings.Builder
	b.Grow(len(token))
	for _, r := range token {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	normalized := strings.Trim(b.String(), "_")
	if normalized == "" {
		return "result"
	}
	return normalized
}

func normalizeIntegrationConfig(cfg map[string]any) {
	input, _ := cfg["input"].(map[string]any)
	if input == nil {
		input = map[string]any{}
	}

	// Common alias: input_mapping -> input
	if len(input) == 0 {
		if mapped, ok := cfg["input_mapping"].(map[string]any); ok && len(mapped) > 0 {
			for key, value := range mapped {
				input[key] = value
			}
		}
	}

	// Common alias: data payload used for DB write actions.
	if values, ok := cfg["data"].(map[string]any); ok && len(values) > 0 {
		if asTrimmedString(input["values"]) == "" {
			input["values"] = values
		}
	}

	// Common alias: table at config root instead of within input.
	if asTrimmedString(input["table"]) == "" {
		if table := asTrimmedString(cfg["table"]); table != "" {
			input["table"] = table
		}
	}

	// Back-compat for placeholder capability naming.
	if asTrimmedString(cfg["connector"]) == "" {
		if capability := asTrimmedString(cfg["capability"]); capability != "" {
			cfg["connector"] = capability
		}
	}

	if len(input) > 0 {
		cfg["input"] = input
	}
}

func normalizeHumanTaskForm(cfg map[string]any, step map[string]any) {
	if asTrimmedString(cfg["assignee"]) == "" {
		if role := asTrimmedString(cfg["assign_to_role"]); role != "" {
			cfg["assignee"] = role
		} else if user := asTrimmedString(cfg["assign_to_user"]); user != "" {
			cfg["assignee"] = user
		}
	}
	if formRaw, ok := cfg["form_schema"]; ok {
		if formSchema, ok := formRaw.(map[string]any); ok {
			normalizeFormSchemaFields(formSchema)
		}
		return
	}
	form, ok := cfg["form"].(map[string]any)
	if !ok {
		return
	}
	formSchema := map[string]any{
		"title": asTrimmedString(form["title"]),
	}
	if formSchema["title"] == "" {
		formSchema["title"] = defaultFormTitle(cfg, step)
	}
	if fields, ok := form["fields"].([]any); ok {
		formSchema["fields"] = normalizeLegacyFields(fields)
	}
	if _, ok := formSchema["actions"]; !ok {
		formSchema["actions"] = []any{
			map[string]any{
				"label": "Submit",
				"value": "submit",
				"style": "primary",
			},
		}
	}
	cfg["form_schema"] = formSchema
	delete(cfg, "form")
}

func normalizeExtractionConfig(cfg map[string]any) {
	if asTrimmedString(cfg["document_ref"]) == "" {
		if documentPath := asTrimmedString(cfg["document_path"]); documentPath != "" {
			cfg["document_ref"] = documentPath
		}
	}
	if asTrimmedString(cfg["schema_name"]) == "" {
		if schemaName := asTrimmedString(cfg["schema"]); schemaName != "" {
			cfg["schema_name"] = schemaName
		}
	}
	if rawOnReview, ok := cfg["on_review"]; ok {
		if _, isObject := rawOnReview.(map[string]any); !isObject {
			stepID := asTrimmedString(rawOnReview)
			if stepID == "" {
				delete(cfg, "on_review")
			} else {
				cfg["on_review"] = map[string]any{
					"task_type": stepID,
				}
			}
		}
	}
	if rawOnReject, ok := cfg["on_reject"]; ok {
		if _, isObject := rawOnReject.(map[string]any); !isObject {
			gotoStep := asTrimmedString(rawOnReject)
			if gotoStep == "" {
				delete(cfg, "on_reject")
			} else {
				cfg["on_reject"] = map[string]any{
					"goto": gotoStep,
				}
			}
		}
	}
	delete(cfg, "document_path")
	delete(cfg, "schema")
}

func normalizeRuleConfig(cfg map[string]any, step map[string]any) {
	if existing, ok := step["outcomes"].(map[string]any); ok && len(existing) > 0 {
		return
	}
	outcomes := make(map[string]any)
	rawOutcomeValues := cfg["outcomes"]
	switch typed := rawOutcomeValues.(type) {
	case []any:
		for _, raw := range typed {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name := asTrimmedString(item["name"])
			if name == "" {
				continue
			}
			targets := normalizeRuleTargets(item)
			if len(targets) == 0 {
				continue
			}
			outcomes[name] = targets
		}
	case map[string]any:
		for name, raw := range typed {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			targets := normalizeRuleTargets(item)
			if len(targets) == 0 {
				continue
			}
			outcomes[name] = targets
		}
	}
	if len(outcomes) > 0 {
		step["outcomes"] = outcomes
	}
}

func normalizeRuleTargets(item map[string]any) []string {
	targets := make([]string, 0)
	if target := asTrimmedString(item["target"]); target != "" {
		targets = append(targets, target)
	}
	if nextStep := asTrimmedString(item["next_step"]); nextStep != "" {
		targets = append(targets, nextStep)
	}
	if gotoStep := asTrimmedString(item["goto"]); gotoStep != "" {
		targets = append(targets, gotoStep)
	}
	if rawTargets, ok := item["targets"].([]any); ok {
		for _, candidate := range rawTargets {
			target := asTrimmedString(candidate)
			if target == "" {
				continue
			}
			targets = append(targets, target)
		}
	}
	return targets
}

func normalizeFormSchemaFields(formSchema map[string]any) {
	fields, ok := formSchema["fields"].([]any)
	if !ok {
		return
	}
	formSchema["fields"] = normalizeLegacyFields(fields)
}

func normalizeLegacyFields(fields []any) []any {
	normalized := make([]any, 0, len(fields))
	for _, raw := range fields {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		bind := asTrimmedString(field["bind"])
		if bind == "" {
			bind = asTrimmedString(field["key"])
		}
		if bind == "" {
			continue
		}
		out := map[string]any{
			"bind": bind,
		}
		if label := asTrimmedString(field["label"]); label != "" {
			out["label"] = label
		}
		if fieldType := asTrimmedString(field["type"]); fieldType != "" {
			out["type"] = fieldType
		}
		if required, ok := field["required"].(bool); ok {
			out["required"] = required
		}
		if options, ok := field["options"]; ok {
			out["options"] = options
		}
		normalized = append(normalized, out)
	}
	return normalized
}

func defaultFormTitle(cfg map[string]any, step map[string]any) string {
	if name := asTrimmedString(cfg["name"]); name != "" {
		return name
	}
	if id := asTrimmedString(step["id"]); id != "" {
		return id
	}
	return "Form"
}

func normalizeBuilderStepType(stepType string) (string, error) {
	raw := strings.ToLower(strings.TrimSpace(stepType))
	switch raw {
	case "human", "human_task", "human-task", "human review":
		return "human_task", nil
	case "ai_agent", "llm_agent", "agent":
		return "agent", nil
	case "ai_component", "ai-component":
		return "ai_component", nil
	case "extraction", "document_extraction", "doc_extraction", "extract":
		return "extraction", nil
	case "connector", "integration_step", "integration":
		return "integration", nil
	case "decision_rule", "rule":
		return "rule", nil
	case "delay", "timer":
		return "timer", nil
	case "notify", "notification":
		return "notification", nil
	case "":
		return "", nil
	default:
		return "", fmt.Errorf("unknown step type %q", stepType)
	}
}

func asTrimmedString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
