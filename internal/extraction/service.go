package extraction

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type repository interface {
	ListSchemas(ctx context.Context, tenantID uuid.UUID) ([]Schema, error)
	GetSchema(ctx context.Context, tenantID, id uuid.UUID) (Schema, error)
	CreateSchema(ctx context.Context, tenantID uuid.UUID, req UpsertSchemaRequest) (Schema, error)
	UpdateSchema(ctx context.Context, tenantID, id uuid.UUID, req UpsertSchemaRequest) (Schema, error)
	DeleteSchema(ctx context.Context, tenantID, id uuid.UUID) error
	GetJob(ctx context.Context, tenantID, id uuid.UUID) (Job, error)
	ListFields(ctx context.Context, tenantID, jobID uuid.UUID) ([]Field, error)
	AcceptJob(ctx context.Context, tenantID, jobID uuid.UUID) error
	RejectJob(ctx context.Context, tenantID, jobID uuid.UUID) error
	ConfirmField(ctx context.Context, tenantID, fieldID uuid.UUID) error
	CorrectField(ctx context.Context, tenantID, fieldID uuid.UUID, correctedValue string, correctedBy uuid.UUID) error
	RejectField(ctx context.Context, tenantID, fieldID uuid.UUID) error
	ListCorrections(ctx context.Context, tenantID uuid.UUID, schemaID *uuid.UUID, since *time.Time) ([]Correction, error)
	GetReviewOutputPath(ctx context.Context, caseID uuid.UUID, stepID string) (string, error)
}

type stepCompleter interface {
	CompleteStep(ctx context.Context, caseID uuid.UUID, stepID string, result *engine.StepResult) error
}

type Service struct {
	repo      repository
	completer stepCompleter
}

func NewService(repo repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetStepCompleter(completer stepCompleter) {
	s.completer = completer
}

func (s *Service) ListSchemas(ctx context.Context, tenantID uuid.UUID) ([]Schema, error) {
	return s.repo.ListSchemas(ctx, tenantID)
}

func (s *Service) GetSchema(ctx context.Context, tenantID, id uuid.UUID) (Schema, error) {
	return s.repo.GetSchema(ctx, tenantID, id)
}

func (s *Service) CreateSchema(ctx context.Context, tenantID uuid.UUID, req UpsertSchemaRequest) (Schema, error) {
	if err := validateUpsert(req); err != nil {
		return Schema{}, err
	}
	return s.repo.CreateSchema(ctx, tenantID, req)
}

func (s *Service) UpdateSchema(ctx context.Context, tenantID, id uuid.UUID, req UpsertSchemaRequest) (Schema, error) {
	if err := validateUpsert(req); err != nil {
		return Schema{}, err
	}
	return s.repo.UpdateSchema(ctx, tenantID, id, req)
}

func (s *Service) DeleteSchema(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteSchema(ctx, tenantID, id)
}

func (s *Service) GetJob(ctx context.Context, tenantID, id uuid.UUID) (Job, error) {
	return s.repo.GetJob(ctx, tenantID, id)
}

func (s *Service) ListFields(ctx context.Context, tenantID, jobID uuid.UUID) ([]Field, error) {
	return s.repo.ListFields(ctx, tenantID, jobID)
}

func (s *Service) AcceptJob(ctx context.Context, tenantID, jobID uuid.UUID) error {
	job, err := s.repo.GetJob(ctx, tenantID, jobID)
	if err != nil {
		return err
	}
	if err := s.repo.AcceptJob(ctx, tenantID, jobID); err != nil {
		return err
	}
	if s.completer == nil {
		return nil
	}
	fields, err := s.repo.ListFields(ctx, tenantID, jobID)
	if err != nil {
		return err
	}
	outputPath, err := s.repo.GetReviewOutputPath(ctx, job.CaseID, job.StepID)
	if err != nil {
		return err
	}
	summary := map[string]any{
		"job_id":     job.ID.String(),
		"status":     "accepted",
		"confidence": floatOrZero(job.Confidence),
		"fields":     summarizeFields(fields),
	}
	outputRaw, _ := json.Marshal(summary)
	patchRaw := json.RawMessage("{}")
	writes := false
	if outputPath != "" {
		patch := map[string]any{}
		setPatchPath(patch, outputPath, summary)
		patchRaw, _ = json.Marshal(patch)
		writes = true
	}
	return s.completer.CompleteStep(ctx, job.CaseID, job.StepID, &engine.StepResult{
		Outcome:        "accept",
		Output:         outputRaw,
		WritesCaseData: writes,
		CaseDataPatch:  patchRaw,
		AuditEventType: "extraction.accepted",
	})
}

func (s *Service) RejectJob(ctx context.Context, tenantID, jobID uuid.UUID) error {
	job, err := s.repo.GetJob(ctx, tenantID, jobID)
	if err != nil {
		return err
	}
	if err := s.repo.RejectJob(ctx, tenantID, jobID); err != nil {
		return err
	}
	if s.completer == nil {
		return nil
	}
	output := map[string]any{
		"job_id":     job.ID.String(),
		"status":     "rejected",
		"confidence": floatOrZero(job.Confidence),
	}
	outputRaw, _ := json.Marshal(output)
	return s.completer.CompleteStep(ctx, job.CaseID, job.StepID, &engine.StepResult{
		Outcome:        "reject",
		Output:         outputRaw,
		AuditEventType: "extraction.rejected",
	})
}

func (s *Service) ConfirmField(ctx context.Context, tenantID, fieldID uuid.UUID) error {
	return s.repo.ConfirmField(ctx, tenantID, fieldID)
}

func (s *Service) CorrectField(ctx context.Context, tenantID, fieldID uuid.UUID, correctedValue string, correctedBy uuid.UUID) error {
	if strings.TrimSpace(correctedValue) == "" {
		return fmt.Errorf("corrected_value is required")
	}
	return s.repo.CorrectField(ctx, tenantID, fieldID, correctedValue, correctedBy)
}

func (s *Service) RejectField(ctx context.Context, tenantID, fieldID uuid.UUID) error {
	return s.repo.RejectField(ctx, tenantID, fieldID)
}

func (s *Service) ListCorrections(ctx context.Context, tenantID uuid.UUID, schemaID *uuid.UUID, since *time.Time) ([]Correction, error) {
	return s.repo.ListCorrections(ctx, tenantID, schemaID, since)
}

func validateUpsert(req UpsertSchemaRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(req.Fields) == 0 {
		return fmt.Errorf("fields is required")
	}
	var parsed any
	if err := json.Unmarshal(req.Fields, &parsed); err != nil {
		return fmt.Errorf("fields must be valid json: %w", err)
	}
	if _, ok := parsed.([]any); !ok {
		return fmt.Errorf("fields must be a json array")
	}
	return nil
}

func IsNotFound(err error) bool {
	return err == sql.ErrNoRows
}

func summarizeFields(fields []Field) map[string]any {
	out := map[string]any{}
	for _, field := range fields {
		value := field.ExtractedValue
		if strings.TrimSpace(field.CorrectedValue) != "" {
			value = field.CorrectedValue
		}
		out[field.FieldName] = map[string]any{
			"value":       value,
			"confidence":  field.Confidence,
			"source_text": field.SourceText,
			"page_number": field.PageNumber,
			"bbox_x":      field.BBoxX,
			"bbox_y":      field.BBoxY,
			"bbox_width":  field.BBoxWidth,
			"bbox_height": field.BBoxHeight,
			"status":      field.Status,
		}
	}
	return out
}

func setPatchPath(root map[string]any, path string, value any) {
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

func floatOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
