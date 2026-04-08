package vault

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Service) Erase(ctx context.Context, tenantID uuid.UUID, req ErasureRequest, actorID uuid.UUID) error {
	caseIDs, err := s.resolveErasureCases(ctx, tenantID, req)
	if err != nil {
		return err
	}
	if len(caseIDs) == 0 {
		return sql.ErrNoRows
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin erasure tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()

	for _, caseID := range caseIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE vault_documents SET deleted_at = now() WHERE tenant_id = $1 AND case_id = $2 AND deleted_at IS NULL`, tenantID, caseID); err != nil {
			return fmt.Errorf("logical delete documents for erasure: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE case_steps SET events = '[]'::jsonb WHERE case_id = $1`, caseID); err != nil {
			return fmt.Errorf("purge case step events for erasure: %w", err)
		}
		if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, "", "system", actorID, "human", "erasure_completed", map[string]any{"case_id": caseID.String(), "data_subject_email": req.DataSubjectEmail}); err != nil {
			return err
		}
	}
	if err := s.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit erasure tx: %w", err)
	}
	for range caseIDs {
		_, _, _ = s.OrphanCleanup(ctx, &tenantID)
	}
	return nil
}

func (s *Service) resolveErasureCases(ctx context.Context, tenantID uuid.UUID, req ErasureRequest) ([]uuid.UUID, error) {
	if req.CaseID != nil && *req.CaseID != uuid.Nil {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cases WHERE id = $1 AND tenant_id = $2)`, *req.CaseID, tenantID).Scan(&exists); err != nil {
			return nil, fmt.Errorf("resolve erasure case by id: %w", err)
		}
		if !exists {
			return nil, sql.ErrNoRows
		}
		return []uuid.UUID{*req.CaseID}, nil
	}
	if strings.TrimSpace(req.DataSubjectEmail) == "" {
		return nil, fmt.Errorf("case_id or data_subject_email is required")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id
FROM cases c
JOIN principals p ON p.id = c.created_by
WHERE c.tenant_id = $1 AND LOWER(COALESCE(p.email,'')) = LOWER($2)
`, tenantID, req.DataSubjectEmail)
	if err != nil {
		return nil, fmt.Errorf("resolve erasure cases by data subject: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]uuid.UUID, 0)
	for rows.Next() {
		var caseID uuid.UUID
		if err := rows.Scan(&caseID); err != nil {
			return nil, fmt.Errorf("scan erasure case id: %w", err)
		}
		out = append(out, caseID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate erasure case ids: %w", err)
	}
	return out, nil
}
