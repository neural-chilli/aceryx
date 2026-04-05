package agentic

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPostgresTraceStore_NilSafe(t *testing.T) {
	var s *PostgresTraceStore
	if err := s.CreateTrace(context.Background(), &ReasoningTrace{ID: uuid.New()}); err != nil {
		t.Fatalf("CreateTrace: %v", err)
	}
	if err := s.UpdateTrace(context.Background(), &ReasoningTrace{ID: uuid.New()}); err != nil {
		t.Fatalf("UpdateTrace: %v", err)
	}
	if err := s.AppendEvent(context.Background(), &ReasoningEvent{ID: uuid.New()}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
}
