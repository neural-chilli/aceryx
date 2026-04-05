package triggers

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPostgresStoreNilSafe(t *testing.T) {
	var s *PostgresStore
	if err := s.Create(context.Background(), &TriggerInstanceRecord{ID: uuid.New()}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Update(context.Background(), &TriggerInstanceRecord{ID: uuid.New()}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := s.ListByTenant(context.Background(), uuid.New()); err != nil {
		t.Fatalf("list by tenant: %v", err)
	}
}
