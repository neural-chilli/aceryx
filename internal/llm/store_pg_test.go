package llm

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestStoreNilSafe(t *testing.T) {
	var s *Store
	if err := s.RecordInvocation(context.Background(), Invocation{}); err != nil {
		t.Fatalf("record invocation: %v", err)
	}
	if err := s.UpdateMonthlyUsage(context.Background(), uuid.New(), 10, 1.2); err != nil {
		t.Fatalf("update monthly usage: %v", err)
	}
	if _, err := s.ListInvocations(context.Background(), uuid.New(), ListOpts{Since: time.Now()}); err != nil {
		t.Fatalf("list invocations: %v", err)
	}
}
