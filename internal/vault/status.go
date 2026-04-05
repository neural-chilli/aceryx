package vault

import (
	"context"
	"fmt"
)

type Status struct {
	BackendType string `json:"backend_type"`
	Healthy     bool   `json:"healthy"`
	ObjectCount int64  `json:"object_count"`
	TotalSize   int64  `json:"total_size"`
	Error       string `json:"error,omitempty"`
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	var out Status
	out.BackendType = s.backendStatus.BackendType
	out.Healthy = s.backendStatus.Healthy
	out.Error = s.backendStatus.Error
	if s.db == nil {
		return out, fmt.Errorf("vault service db is not configured")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(size_bytes),0) FROM vault_documents WHERE deleted_at IS NULL`).Scan(&out.ObjectCount, &out.TotalSize); err != nil {
		return Status{}, fmt.Errorf("load vault status counters: %w", err)
	}
	return out, nil
}
