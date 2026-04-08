package extraction

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

type stepConfig struct {
	DocumentPath        string  `json:"document_path"`
	DocumentRef         string  `json:"document_ref"`
	Schema              string  `json:"schema"`
	SchemaName          string  `json:"schema_name"`
	SchemaID            string  `json:"schema_id"`
	Model               string  `json:"model"`
	AutoAcceptThreshold float64 `json:"auto_accept_threshold"`
	ReviewThreshold     float64 `json:"review_threshold"`
	OutputPath          string  `json:"output_path"`
	OnReview            struct {
		TaskType     string `json:"task_type"`
		AssigneeRole string `json:"assignee_role"`
		SLAHours     int    `json:"sla_hours"`
	} `json:"on_review"`
	OnReject struct {
		Goto string `json:"goto"`
	} `json:"on_reject"`
}

type schemaField struct {
	Name string `json:"name"`
}

type extractedField struct {
	Value      string
	Confidence float64
	SourceText string
	PageNumber *int
	BBoxX      *float64
	BBoxY      *float64
	BBoxWidth  *float64
	BBoxHeight *float64
}

type taskCreator interface {
	CreateTaskFromActivation(ctx context.Context, caseID uuid.UUID, stepID string, cfg tasks.AssignmentConfig) error
}

type StepExecutor struct {
	db    *sql.DB
	tasks taskCreator
}

func NewStepExecutor(db *sql.DB, tasks taskCreator) *StepExecutor {
	return &StepExecutor{db: db, tasks: tasks}
}

func (s *StepExecutor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, raw json.RawMessage) (*engine.StepResult, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("extraction step executor not configured")
	}
	cfg, err := decodeStepConfig(raw)
	if err != nil {
		return nil, err
	}

	tenantID, caseData, err := s.loadCaseContext(ctx, caseID)
	if err != nil {
		return nil, err
	}
	documentID, err := resolveDocumentID(cfg.DocumentPath, caseData)
	if err != nil {
		return nil, err
	}
	schemaID, schemaFields, err := s.resolveSchema(ctx, tenantID, cfg.Schema)
	if err != nil {
		return nil, err
	}
	documentExtracted, err := s.loadDocumentExtractedData(ctx, tenantID, documentID)
	if err != nil {
		return nil, err
	}

	fieldValues := buildFieldValues(schemaFields, documentExtracted)
	overallConfidence := minConfidence(fieldValues)
	status := routeStatus(overallConfidence, cfg.AutoAcceptThreshold, cfg.ReviewThreshold)
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-5.4"
	}

	jobID, err := s.insertJobAndFields(ctx, tenantID, caseID, stepID, documentID, schemaID, model, status, overallConfidence, fieldValues, documentExtracted)
	if err != nil {
		return nil, err
	}

	summary := map[string]any{
		"job_id":     jobID.String(),
		"status":     status,
		"confidence": overallConfidence,
		"schema_id":  schemaID.String(),
		"model":      model,
		"fields":     flattenFieldValues(fieldValues),
	}
	outputRaw, _ := json.Marshal(summary)
	eventRaw, _ := json.Marshal(map[string]any{
		"type":       "extraction_execution",
		"job_id":     jobID.String(),
		"status":     status,
		"step_id":    stepID,
		"confidence": overallConfidence,
		"model":      model,
	})

	if status == "review" {
		if err := s.createReviewTask(ctx, caseID, stepID, cfg, jobID, overallConfidence); err != nil {
			return nil, err
		}
		return nil, engine.ErrStepAwaitingReview
	}

	if status == "accepted" {
		patch := map[string]any{}
		setPath(patch, cfg.OutputPath, summary)
		patchRaw, _ := json.Marshal(patch)
		return &engine.StepResult{
			Outcome:        "accept",
			Output:         outputRaw,
			WritesCaseData: true,
			CaseDataPatch:  patchRaw,
			ExecutionEvent: eventRaw,
			AuditEventType: "extraction.accepted",
		}, nil
	}

	return &engine.StepResult{
		Outcome:        "reject",
		Output:         outputRaw,
		ExecutionEvent: eventRaw,
		AuditEventType: "extraction.rejected",
	}, nil
}

func decodeStepConfig(raw json.RawMessage) (stepConfig, error) {
	cfg := stepConfig{
		AutoAcceptThreshold: 0.85,
		ReviewThreshold:     0,
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return stepConfig{}, fmt.Errorf("decode extraction step config: %w", err)
		}
	}
	if strings.TrimSpace(cfg.DocumentPath) == "" {
		cfg.DocumentPath = strings.TrimSpace(cfg.DocumentRef)
	}
	if strings.TrimSpace(cfg.Schema) == "" {
		if strings.TrimSpace(cfg.SchemaName) != "" {
			cfg.Schema = strings.TrimSpace(cfg.SchemaName)
		} else {
			cfg.Schema = strings.TrimSpace(cfg.SchemaID)
		}
	}
	if strings.TrimSpace(cfg.DocumentPath) == "" {
		return stepConfig{}, fmt.Errorf("extraction step config missing document_path/document_ref")
	}
	if strings.TrimSpace(cfg.Schema) == "" {
		return stepConfig{}, fmt.Errorf("extraction step config missing schema/schema_name/schema_id")
	}
	if strings.TrimSpace(cfg.OutputPath) == "" {
		return stepConfig{}, fmt.Errorf("extraction step config missing output_path")
	}
	return cfg, nil
}

func (s *StepExecutor) loadCaseContext(ctx context.Context, caseID uuid.UUID) (uuid.UUID, map[string]any, error) {
	var (
		tenantID uuid.UUID
		rawData  []byte
	)
	if err := s.db.QueryRowContext(ctx, `
SELECT tenant_id, data
FROM cases
WHERE id = $1
`, caseID).Scan(&tenantID, &rawData); err != nil {
		return uuid.Nil, nil, fmt.Errorf("load extraction case context: %w", err)
	}
	data := map[string]any{}
	if len(rawData) > 0 {
		if err := json.Unmarshal(rawData, &data); err != nil {
			return uuid.Nil, nil, fmt.Errorf("decode extraction case data: %w", err)
		}
	}
	return tenantID, data, nil
}

func resolveDocumentID(path string, caseData map[string]any) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(path)
	if id, err := uuid.Parse(trimmed); err == nil {
		return id, nil
	}
	value, ok := lookupPath(caseData, normalizeCaseDataPath(trimmed))
	if !ok {
		return uuid.Nil, fmt.Errorf("document_path %q not found in case data", path)
	}
	id, err := uuid.Parse(strings.TrimSpace(stringify(value)))
	if err != nil {
		return uuid.Nil, fmt.Errorf("document_path %q did not resolve to uuid", path)
	}
	return id, nil
}

func (s *StepExecutor) resolveSchema(ctx context.Context, tenantID uuid.UUID, schema string) (uuid.UUID, []schemaField, error) {
	schema = strings.TrimSpace(schema)
	var (
		id       uuid.UUID
		rawField []byte
	)
	if parsed, err := uuid.Parse(schema); err == nil {
		if err := s.db.QueryRowContext(ctx, `
SELECT id, fields
FROM extraction_schemas
WHERE tenant_id = $1
  AND id = $2
`, tenantID, parsed).Scan(&id, &rawField); err != nil {
			return uuid.Nil, nil, fmt.Errorf("load extraction schema by id: %w", err)
		}
	} else {
		if err := s.db.QueryRowContext(ctx, `
SELECT id, fields
FROM extraction_schemas
WHERE tenant_id = $1
  AND name = $2
`, tenantID, schema).Scan(&id, &rawField); err != nil {
			return uuid.Nil, nil, fmt.Errorf("load extraction schema by name: %w", err)
		}
	}
	var fields []schemaField
	if err := json.Unmarshal(rawField, &fields); err != nil {
		return uuid.Nil, nil, fmt.Errorf("decode extraction schema fields: %w", err)
	}
	return id, fields, nil
}

func (s *StepExecutor) loadDocumentExtractedData(ctx context.Context, tenantID, documentID uuid.UUID) (map[string]any, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(extracted_data, '{}'::jsonb)
FROM vault_documents
WHERE tenant_id = $1
  AND id = $2
  AND deleted_at IS NULL
`, tenantID, documentID).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("load vault extracted_data: %w", err)
	}
	data := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("decode vault extracted_data: %w", err)
		}
	}
	return data, nil
}

func buildFieldValues(schemaFields []schemaField, extracted map[string]any) map[string]extractedField {
	names := make([]string, 0, len(schemaFields))
	for _, field := range schemaFields {
		name := strings.TrimSpace(field.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		for key := range extracted {
			names = append(names, key)
		}
		sort.Strings(names)
	}
	out := make(map[string]extractedField, len(names))
	for _, name := range names {
		out[name] = parseExtractedField(extracted[name])
	}
	return out
}

func parseExtractedField(value any) extractedField {
	field := extractedField{Confidence: 0}
	switch typed := value.(type) {
	case map[string]any:
		field.Value = stringify(typed["value"])
		field.Confidence = asFloat(typed["confidence"])
		field.SourceText = stringify(typed["source_text"])
		field.PageNumber = asIntPtr(typed["page_number"])
		field.BBoxX = asFloatPtr(typed["bbox_x"])
		field.BBoxY = asFloatPtr(typed["bbox_y"])
		field.BBoxWidth = asFloatPtr(typed["bbox_width"])
		field.BBoxHeight = asFloatPtr(typed["bbox_height"])
	default:
		field.Value = stringify(value)
	}
	if field.SourceText == "" || field.BBoxX == nil || field.BBoxY == nil || field.BBoxWidth == nil || field.BBoxHeight == nil {
		field.Confidence = 0
	}
	return field
}

func minConfidence(fields map[string]extractedField) float64 {
	if len(fields) == 0 {
		return 0
	}
	min := 1.0
	for _, field := range fields {
		if field.Confidence < min {
			min = field.Confidence
		}
	}
	return min
}

func routeStatus(confidence float64, autoAccept float64, reviewThreshold float64) string {
	if confidence >= autoAccept {
		return "accepted"
	}
	if confidence < reviewThreshold {
		return "rejected"
	}
	return "review"
}

func (s *StepExecutor) insertJobAndFields(ctx context.Context, tenantID, caseID uuid.UUID, stepID string, documentID, schemaID uuid.UUID, model string, status string, confidence float64, fields map[string]extractedField, extractedData map[string]any) (uuid.UUID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin extraction insert tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rawExtracted, _ := json.Marshal(extractedData)
	rawResponse, _ := json.Marshal(map[string]any{"source": "vault_document.extracted_data"})
	var jobID uuid.UUID
	if err := tx.QueryRowContext(ctx, `
INSERT INTO extraction_jobs (
    tenant_id,
    case_id,
    step_id,
    document_id,
    schema_id,
    model_used,
    status,
    confidence,
    extracted_data,
    raw_response
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb)
RETURNING id
`, tenantID, caseID, stepID, documentID, schemaID, model, status, confidence, string(rawExtracted), string(rawResponse)).Scan(&jobID); err != nil {
		return uuid.Nil, fmt.Errorf("insert extraction job: %w", err)
	}

	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field := fields[name]
		if _, err := tx.ExecContext(ctx, `
INSERT INTO extraction_fields (
    job_id,
    field_name,
    extracted_value,
    confidence,
    source_text,
    page_number,
    bbox_x,
    bbox_y,
    bbox_width,
    bbox_height,
    status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'extracted')
`, jobID, name, field.Value, field.Confidence, nullString(field.SourceText), field.PageNumber, field.BBoxX, field.BBoxY, field.BBoxWidth, field.BBoxHeight); err != nil {
			return uuid.Nil, fmt.Errorf("insert extraction field %q: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("commit extraction job insert: %w", err)
	}
	return jobID, nil
}

func (s *StepExecutor) createReviewTask(ctx context.Context, caseID uuid.UUID, stepID string, cfg stepConfig, jobID uuid.UUID, confidence float64) error {
	if s.tasks == nil {
		return fmt.Errorf("task service not configured for extraction review")
	}
	role := strings.TrimSpace(cfg.OnReview.AssigneeRole)
	if role == "" {
		role = "case_worker"
	}
	taskType := strings.TrimSpace(cfg.OnReview.TaskType)
	if taskType == "" {
		taskType = "extraction_review"
	}
	sla := cfg.OnReview.SLAHours
	if sla <= 0 {
		sla = 4
	}
	return s.tasks.CreateTaskFromActivation(ctx, caseID, stepID, tasks.AssignmentConfig{
		AssignToRole: role,
		SLAHours:     sla,
		Form:         taskType,
		Outcomes:     []string{"accept", "correct", "reject"},
		Metadata: map[string]any{
			"extraction_review": map[string]any{
				"job_id":      jobID.String(),
				"confidence":  confidence,
				"output_path": cfg.OutputPath,
			},
		},
	})
}

func flattenFieldValues(fields map[string]extractedField) map[string]any {
	out := map[string]any{}
	for name, field := range fields {
		out[name] = map[string]any{
			"value":       field.Value,
			"confidence":  field.Confidence,
			"source_text": field.SourceText,
			"page_number": field.PageNumber,
			"bbox_x":      field.BBoxX,
			"bbox_y":      field.BBoxY,
			"bbox_width":  field.BBoxWidth,
			"bbox_height": field.BBoxHeight,
		}
	}
	return out
}

func normalizeCaseDataPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "case.data.")
	path = strings.TrimPrefix(path, "case.data")
	path = strings.TrimPrefix(path, "data.")
	return path
}

func lookupPath(root any, path string) (any, bool) {
	if strings.TrimSpace(path) == "" {
		return root, true
	}
	segments := parseSegments(path)
	current := root
	for _, segment := range segments {
		switch container := current.(type) {
		case map[string]any:
			next, ok := container[segment.Key]
			if !ok {
				return nil, false
			}
			current = next
		default:
			return nil, false
		}
		if segment.Index != nil {
			arr, ok := current.([]any)
			if !ok {
				return nil, false
			}
			if *segment.Index < 0 || *segment.Index >= len(arr) {
				return nil, false
			}
			current = arr[*segment.Index]
		}
	}
	return current, true
}

type pathSegment struct {
	Key   string
	Index *int
}

func parseSegments(path string) []pathSegment {
	parts := strings.Split(path, ".")
	out := make([]pathSegment, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		open := strings.Index(part, "[")
		close := strings.Index(part, "]")
		if open > 0 && close > open {
			key := strings.TrimSpace(part[:open])
			rawIdx := strings.TrimSpace(part[open+1 : close])
			if idx, err := strconv.Atoi(rawIdx); err == nil {
				i := idx
				out = append(out, pathSegment{Key: key, Index: &i})
				continue
			}
		}
		out = append(out, pathSegment{Key: part})
	}
	return out
}

func setPath(root map[string]any, path string, value any) {
	path = strings.TrimSpace(path)
	if path == "" {
		root["result"] = value
		return
	}
	path = strings.TrimPrefix(path, "case.data.")
	path = strings.TrimPrefix(path, "case.data")
	parts := strings.Split(path, ".")
	current := root
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

func asFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		n, _ := v.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return n
	default:
		return 0
	}
}

func asFloatPtr(value any) *float64 {
	switch value.(type) {
	case nil:
		return nil
	}
	f := asFloat(value)
	return &f
}

func asIntPtr(value any) *int {
	switch v := value.(type) {
	case int:
		n := v
		return &n
	case int64:
		n := int(v)
		return &n
	case float64:
		n := int(v)
		return &n
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return nil
		}
		n := int(i)
		return &n
	default:
		return nil
	}
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(raw)
	}
}

func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
