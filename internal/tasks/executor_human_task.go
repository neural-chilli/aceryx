package tasks

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
)

// HumanTaskExecutor activates a long-running human task and returns immediately.
type HumanTaskExecutor struct {
	svc *TaskService
}

func NewHumanTaskExecutor(svc *TaskService) *HumanTaskExecutor {
	return &HumanTaskExecutor{svc: svc}
}

func (e *HumanTaskExecutor) Execute(ctx context.Context, caseID uuid.UUID, stepID string, config json.RawMessage) (*engine.StepResult, error) {
	cfg := AssignmentConfig{}
	if len(config) > 0 {
		_ = json.Unmarshal(config, &cfg)
	}
	if err := e.svc.CreateTaskFromActivation(ctx, caseID, stepID, cfg); err != nil {
		return nil, err
	}
	return nil, engine.ErrStepAwaitingReview
}
