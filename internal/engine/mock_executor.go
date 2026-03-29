package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

type MockExecution struct {
	Result *StepResult
	Err    error
}

// MockExecutor is deterministic and test-oriented.
type MockExecutor struct {
	mu      sync.Mutex
	results map[string][]MockExecution
}

func NewMockExecutor(results map[string][]MockExecution) *MockExecutor {
	cloned := make(map[string][]MockExecution, len(results))
	for k, v := range results {
		vv := make([]MockExecution, len(v))
		copy(vv, v)
		cloned[k] = vv
	}
	return &MockExecutor{results: cloned}
}

func (m *MockExecutor) Execute(_ context.Context, _ uuid.UUID, stepID string, _ json.RawMessage) (*StepResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, ok := m.results[stepID]
	if !ok || len(queue) == 0 {
		return nil, fmt.Errorf("mock executor has no result configured for step %q", stepID)
	}

	next := queue[0]
	m.results[stepID] = queue[1:]
	if next.Err != nil {
		return nil, next.Err
	}
	return next.Result, nil
}
