package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func (s *Service) DownloadURL(ctx context.Context, tenantID, docID uuid.UUID, expirySeconds int) (SignedDocumentURL, error) {
	var caseID uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT case_id FROM vault_documents WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, docID, tenantID).Scan(&caseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SignedDocumentURL{}, sql.ErrNoRows
		}
		return SignedDocumentURL{}, fmt.Errorf("load case for document: %w", err)
	}
	return s.SignedURL(ctx, tenantID, caseID, docID, expirySeconds)
}
