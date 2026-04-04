package backup

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (s *Service) collectMetadata(ctx context.Context, tenantID *uuid.UUID) (Metadata, error) {
	meta := Metadata{
		Version:       BackupFormatVersion,
		AceryxVersion: AceryxVersion,
		CreatedAt:     s.now(),
		TenantFilter:  tenantID,
	}

	if err := s.db.QueryRowContext(ctx, `SELECT current_setting('server_version')`).Scan(&meta.PostgresVersion); err != nil {
		return Metadata{}, fmt.Errorf("query postgres version: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&meta.SchemaVersion); err != nil {
		return Metadata{}, fmt.Errorf("query schema version: %w", err)
	}

	if tenantID == nil {
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases`).Scan(&meta.CaseCount); err != nil {
			return Metadata{}, fmt.Errorf("query case count: %w", err)
		}
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE deleted_at IS NULL`).Scan(&meta.DocumentCount); err != nil {
			return Metadata{}, fmt.Errorf("query document count: %w", err)
		}
		return meta, nil
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases WHERE tenant_id = $1`, *tenantID).Scan(&meta.CaseCount); err != nil {
		return Metadata{}, fmt.Errorf("query tenant case count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vault_documents WHERE tenant_id = $1 AND deleted_at IS NULL`, *tenantID).Scan(&meta.DocumentCount); err != nil {
		return Metadata{}, fmt.Errorf("query tenant document count: %w", err)
	}
	return meta, nil
}
