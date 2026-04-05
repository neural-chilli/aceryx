package vault

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

func (s *Service) OrphanCleanup(ctx context.Context, tenantID *uuid.UUID) (int, int64, error) {
	if tenantID == nil {
		return s.orphanCleanupAllTenants(ctx)
	}

	query := `
SELECT DISTINCT vd1.tenant_id, vd1.content_hash, vd1.storage_uri
FROM vault_documents vd1
WHERE vd1.deleted_at IS NOT NULL
  AND vd1.tenant_id = $1
  AND NOT EXISTS (
  SELECT 1 FROM vault_documents vd2
  WHERE vd2.content_hash = vd1.content_hash
    AND vd2.tenant_id = vd1.tenant_id
    AND vd2.deleted_at IS NULL
)`

	rows, err := s.db.QueryContext(ctx, query, *tenantID)
	if err != nil {
		return 0, 0, fmt.Errorf("query orphan documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type orphan struct {
		TenantID    uuid.UUID
		ContentHash string
		StorageURI  string
	}
	orphans := make([]orphan, 0)
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.TenantID, &o.ContentHash, &o.StorageURI); err != nil {
			return 0, 0, fmt.Errorf("scan orphan row: %w", err)
		}
		orphans = append(orphans, o)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("iterate orphan rows: %w", err)
	}

	filesDeleted := 0
	var bytesDeleted int64
	for _, o := range orphans {
		if data, err := s.store.Get(o.StorageURI); err == nil {
			bytesDeleted += int64(len(data))
		}
		_ = s.store.Delete(o.StorageURI)
		if _, err := s.db.ExecContext(ctx, `DELETE FROM vault_documents WHERE tenant_id = $1 AND content_hash = $2 AND deleted_at IS NOT NULL`, o.TenantID, o.ContentHash); err != nil {
			return filesDeleted, bytesDeleted, fmt.Errorf("delete orphan metadata rows: %w", err)
		}
		filesDeleted++
	}
	return filesDeleted, bytesDeleted, nil
}

func (s *Service) orphanCleanupAllTenants(ctx context.Context) (int, int64, error) {
	tenantIDs, err := s.listTenantsWithDeletedDocuments(ctx)
	if err != nil {
		return 0, 0, err
	}
	totalFiles := 0
	var totalBytes int64
	for _, tid := range tenantIDs {
		filesDeleted, bytesDeleted, cErr := s.OrphanCleanup(ctx, &tid)
		if cErr != nil {
			return totalFiles, totalBytes, cErr
		}
		totalFiles += filesDeleted
		totalBytes += bytesDeleted
	}
	return totalFiles, totalBytes, nil
}

func (s *Service) listTenantsWithDeletedDocuments(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT tenant_id
FROM vault_documents
WHERE deleted_at IS NOT NULL
`)
	if err != nil {
		return nil, fmt.Errorf("query tenants with deleted vault docs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tenantIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var tid uuid.UUID
		if err := rows.Scan(&tid); err != nil {
			return nil, fmt.Errorf("scan tenant for orphan cleanup: %w", err)
		}
		tenantIDs = append(tenantIDs, tid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenants for orphan cleanup: %w", err)
	}
	return tenantIDs, nil
}

func (s *Service) StartOrphanCleanupTicker(ctx context.Context) {
	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, _, err := s.OrphanCleanup(ctx, nil); err != nil {
				slog.WarnContext(ctx, "vault orphan cleanup tick failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
