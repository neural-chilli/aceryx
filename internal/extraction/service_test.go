package extraction

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type fakeRepo struct {
	job        Job
	fields     []Field
	outputPath string
}

func (f *fakeRepo) ListSchemas(context.Context, uuid.UUID) ([]Schema, error) {
	return nil, nil
}
func (f *fakeRepo) GetSchema(context.Context, uuid.UUID, uuid.UUID) (Schema, error) {
	return Schema{}, nil
}
func (f *fakeRepo) CreateSchema(context.Context, uuid.UUID, UpsertSchemaRequest) (Schema, error) {
	return Schema{}, nil
}
func (f *fakeRepo) UpdateSchema(context.Context, uuid.UUID, uuid.UUID, UpsertSchemaRequest) (Schema, error) {
	return Schema{}, nil
}
func (f *fakeRepo) DeleteSchema(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeRepo) GetJob(context.Context, uuid.UUID, uuid.UUID) (Job, error) {
	return f.job, nil
}
func (f *fakeRepo) ListFields(context.Context, uuid.UUID, uuid.UUID) ([]Field, error) {
	return f.fields, nil
}
func (f *fakeRepo) AcceptJob(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeRepo) RejectJob(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeRepo) ConfirmField(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeRepo) CorrectField(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID) error {
	return nil
}
func (f *fakeRepo) RejectField(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeRepo) ListCorrections(context.Context, uuid.UUID, *uuid.UUID, *time.Time) ([]Correction, error) {
	return nil, nil
}
func (f *fakeRepo) GetReviewOutputPath(context.Context, uuid.UUID, string) (string, error) {
	return f.outputPath, nil
}

type fakeCompleter struct {
	result *engine.StepResult
}

func (f *fakeCompleter) CompleteStep(_ context.Context, _ uuid.UUID, _ string, result *engine.StepResult) error {
	f.result = result
	return nil
}

func TestServiceAcceptJob_CompletesStepWithPatch(t *testing.T) {
	jobID := uuid.New()
	caseID := uuid.New()
	tenantID := uuid.New()
	repo := &fakeRepo{
		job: Job{
			ID:         jobID,
			CaseID:     caseID,
			StepID:     "extract_1",
			Confidence: float64Ptr(0.8),
		},
		fields: []Field{
			{FieldName: "company_number", ExtractedValue: "A", CorrectedValue: "B", Confidence: 0.8, Status: "corrected"},
		},
		outputPath: "case.data.extracted",
	}
	completer := &fakeCompleter{}
	svc := NewService(repo)
	svc.SetStepCompleter(completer)

	if err := svc.AcceptJob(context.Background(), tenantID, jobID); err != nil {
		t.Fatalf("accept job: %v", err)
	}
	if completer.result == nil {
		t.Fatalf("expected step completion")
	}
	if !completer.result.WritesCaseData || completer.result.Outcome != "accept" {
		t.Fatalf("unexpected completion result: %#v", completer.result)
	}
	var patch map[string]any
	if err := json.Unmarshal(completer.result.CaseDataPatch, &patch); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if _, ok := patch["extracted"]; !ok {
		t.Fatalf("expected extracted patch path, got %#v", patch)
	}
}

func TestServiceRejectJob_CompletesStepWithoutPatch(t *testing.T) {
	jobID := uuid.New()
	caseID := uuid.New()
	tenantID := uuid.New()
	repo := &fakeRepo{
		job: Job{
			ID:         jobID,
			CaseID:     caseID,
			StepID:     "extract_1",
			Confidence: float64Ptr(0.2),
		},
	}
	completer := &fakeCompleter{}
	svc := NewService(repo)
	svc.SetStepCompleter(completer)

	if err := svc.RejectJob(context.Background(), tenantID, jobID); err != nil {
		t.Fatalf("reject job: %v", err)
	}
	if completer.result == nil {
		t.Fatalf("expected step completion")
	}
	if completer.result.WritesCaseData || completer.result.Outcome != "reject" {
		t.Fatalf("unexpected completion result: %#v", completer.result)
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}
