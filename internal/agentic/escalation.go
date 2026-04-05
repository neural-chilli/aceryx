package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

type EscalationTaskData struct {
	Goal             string           `json:"goal"`
	Conclusion       json.RawMessage  `json:"conclusion"`
	Confidence       float64          `json:"confidence"`
	ReasoningTrace   []ReasoningEvent `json:"reasoning_trace,omitempty"`
	CaseDataSnapshot json.RawMessage  `json:"case_data_snapshot"`
}

type taskCreator interface {
	CreateTaskFromActivation(ctx context.Context, caseID uuid.UUID, stepID string, cfg tasks.AssignmentConfig) error
}

func createEscalationTask(
	ctx context.Context,
	tasksSvc taskCreator,
	tenantID uuid.UUID,
	caseID uuid.UUID,
	stepID string,
	goal string,
	caseData json.RawMessage,
	config EscalationConfig,
	result RunResult,
	events []*ReasoningEvent,
) error {
	if tasksSvc == nil {
		return fmt.Errorf("task service not configured for agentic escalation")
	}
	conf := 0.0
	if result.Confidence != nil {
		conf = *result.Confidence
	}
	taskData := map[string]any{
		"goal":               goal,
		"conclusion":         json.RawMessage(result.Conclusion),
		"confidence":         conf,
		"case_data_snapshot": json.RawMessage(caseData),
	}
	if config.IncludeTrace {
		trace := make([]ReasoningEvent, 0, len(events))
		for _, event := range events {
			if event == nil {
				continue
			}
			trace = append(trace, *event)
		}
		taskData["reasoning_trace"] = trace
	}
	assignRole := strings.TrimSpace(config.EscalateTo)
	if assignRole == "" {
		assignRole = "case_worker"
	}
	return tasksSvc.CreateTaskFromActivation(ctx, caseID, stepID, tasks.AssignmentConfig{
		AssignToRole: assignRole,
		Form:         "agentic_review",
		Outcomes:     []string{"accept", "modify", "reject"},
		Metadata: map[string]any{
			"agentic_review": taskData,
			"tenant_id":      tenantID.String(),
		},
	})
}
