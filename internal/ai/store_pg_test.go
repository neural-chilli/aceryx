package ai

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestStoreNilSafe(t *testing.T) {
	var s *Store
	if err := s.Create(context.Background(), uuid.New(), &AIComponentDef{}, uuid.New()); err != nil {
		t.Fatalf("create nil-safe: %v", err)
	}
	if err := s.Update(context.Background(), uuid.New(), &AIComponentDef{}); err != nil {
		t.Fatalf("update nil-safe: %v", err)
	}
	if err := s.Delete(context.Background(), uuid.New(), "x"); err != nil {
		t.Fatalf("delete nil-safe: %v", err)
	}
	if _, err := s.ListByTenant(context.Background(), uuid.New()); err != nil {
		t.Fatalf("list nil-safe: %v", err)
	}
}
