package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func (s *Service) Upload(ctx context.Context, tenantID uuid.UUID, in UploadInput) (Document, error) {
	if in.CaseID == uuid.Nil {
		return Document{}, fmt.Errorf("case id is required")
	}
	if len(in.Data) == 0 {
		return Document{}, fmt.Errorf("document data is empty")
	}
	if strings.TrimSpace(in.Filename) == "" {
		return Document{}, fmt.Errorf("filename is required")
	}
	if strings.TrimSpace(in.MimeType) == "" {
		in.MimeType = detectMime(in.Filename)
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cases WHERE id = $1 AND tenant_id = $2)`, in.CaseID, tenantID).Scan(&exists); err != nil {
		return Document{}, fmt.Errorf("verify case for document upload: %w", err)
	}
	if !exists {
		return Document{}, sql.ErrNoRows
	}

	hash := ContentHash(in.Data)
	ext := strings.TrimPrefix(filepath.Ext(in.Filename), ".")
	metaJSON, _ := json.Marshal(in.Metadata)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, fmt.Errorf("begin upload tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()

	var storageURI string
	_ = tx.QueryRowContext(ctx, `
SELECT storage_uri
FROM vault_documents
WHERE tenant_id = $1 AND content_hash = $2
LIMIT 1
`, tenantID, hash).Scan(&storageURI)

	if storageURI == "" {
		storageURI, err = s.store.Put(tenantID.String(), hash, ext, in.Data)
		if err != nil {
			return Document{}, err
		}
	}

	var doc Document
	err = tx.QueryRowContext(ctx, `
INSERT INTO vault_documents (tenant_id, case_id, step_id, filename, mime_type, size_bytes, content_hash, storage_uri, uploaded_by, metadata)
VALUES ($1, $2, NULLIF($3,''), $4, $5, $6, $7, $8, $9, $10::jsonb)
RETURNING id, case_id, COALESCE(step_id,''), filename, mime_type, size_bytes, content_hash, uploaded_by, uploaded_at, COALESCE(metadata, '{}'::jsonb)
`, tenantID, in.CaseID, in.StepID, in.Filename, in.MimeType, len(in.Data), hash, storageURI, in.UploadedBy, string(metaJSON)).Scan(
		&doc.ID,
		&doc.CaseID,
		&doc.StepID,
		&doc.Filename,
		&doc.MimeType,
		&doc.SizeBytes,
		&doc.ContentHash,
		&doc.UploadedBy,
		&doc.UploadedAt,
		&doc.Metadata,
	)
	if err != nil {
		return Document{}, fmt.Errorf("insert vault metadata row: %w", err)
	}
	if err := s.auditSvc.RecordCaseEventTx(ctx, tx, in.CaseID, in.StepID, "document", in.UploadedBy, "human", "uploaded", map[string]any{"document_id": doc.ID.String(), "filename": in.Filename, "size_bytes": doc.SizeBytes, "content_hash": hash}); err != nil {
		return Document{}, err
	}
	if err := s.auditSvc.CommitTx(tx); err != nil {
		return Document{}, fmt.Errorf("commit upload tx: %w", err)
	}
	doc.DisplayMode = DisplayModeForMime(doc.MimeType)
	return doc, nil
}

func (s *Service) List(ctx context.Context, tenantID, caseID uuid.UUID) ([]Document, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, case_id, COALESCE(step_id,''), filename, mime_type, size_bytes, content_hash, uploaded_by, uploaded_at, COALESCE(metadata, '{}'::jsonb)
FROM vault_documents
WHERE tenant_id = $1 AND case_id = $2 AND deleted_at IS NULL
ORDER BY uploaded_at DESC
`, tenantID, caseID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Document, 0)
	for rows.Next() {
		var doc Document
		if err := rows.Scan(&doc.ID, &doc.CaseID, &doc.StepID, &doc.Filename, &doc.MimeType, &doc.SizeBytes, &doc.ContentHash, &doc.UploadedBy, &doc.UploadedAt, &doc.Metadata); err != nil {
			return nil, fmt.Errorf("scan document row: %w", err)
		}
		doc.DisplayMode = DisplayModeForMime(doc.MimeType)
		out = append(out, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}
	return out, nil
}

func (s *Service) Download(ctx context.Context, tenantID, caseID, docID, actorID uuid.UUID) (Document, []byte, error) {
	doc, storageURI, err := s.getDocumentRecord(ctx, tenantID, caseID, docID)
	if err != nil {
		return Document{}, nil, err
	}
	blob, err := s.store.Get(storageURI)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Document{}, nil, sql.ErrNoRows
		}
		return Document{}, nil, err
	}
	if err := s.recordDownloadAudit(ctx, caseID, doc.StepID, actorID, doc.ID, doc.Filename); err != nil {
		return Document{}, nil, err
	}
	doc.DisplayMode = DisplayModeForMime(doc.MimeType)
	return doc, blob, nil
}

func (s *Service) Delete(ctx context.Context, tenantID, caseID, docID, actorID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete document tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()

	var stepID, filename string
	err = tx.QueryRowContext(ctx, `
UPDATE vault_documents
SET deleted_at = now()
WHERE id = $1 AND tenant_id = $2 AND case_id = $3 AND deleted_at IS NULL
RETURNING COALESCE(step_id,''), filename
`, docID, tenantID, caseID).Scan(&stepID, &filename)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("logical delete document: %w", err)
	}
	if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "document", actorID, "human", "deleted", map[string]any{"document_id": docID.String(), "filename": filename}); err != nil {
		return err
	}
	if err := s.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit delete document tx: %w", err)
	}
	return nil
}

func (s *Service) getDocumentRecord(ctx context.Context, tenantID, caseID, docID uuid.UUID) (Document, string, error) {
	var (
		doc        Document
		storageURI string
	)
	err := s.db.QueryRowContext(ctx, `
SELECT id, case_id, COALESCE(step_id,''), filename, mime_type, size_bytes, content_hash, uploaded_by, uploaded_at, COALESCE(metadata, '{}'::jsonb), storage_uri
FROM vault_documents
WHERE id = $1 AND tenant_id = $2 AND case_id = $3 AND deleted_at IS NULL
`, docID, tenantID, caseID).Scan(&doc.ID, &doc.CaseID, &doc.StepID, &doc.Filename, &doc.MimeType, &doc.SizeBytes, &doc.ContentHash, &doc.UploadedBy, &doc.UploadedAt, &doc.Metadata, &storageURI)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Document{}, "", sql.ErrNoRows
		}
		return Document{}, "", fmt.Errorf("load document row: %w", err)
	}
	return doc, storageURI, nil
}

func (s *Service) recordDownloadAudit(ctx context.Context, caseID uuid.UUID, stepID string, actorID uuid.UUID, docID uuid.UUID, filename string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin download audit tx: %w", err)
	}
	defer func() { _ = s.auditSvc.RollbackTx(tx) }()
	if err := s.auditSvc.RecordCaseEventTx(ctx, tx, caseID, stepID, "document", actorID, "human", "downloaded", map[string]any{"document_id": docID.String(), "filename": filename}); err != nil {
		return err
	}
	if err := s.auditSvc.CommitTx(tx); err != nil {
		return fmt.Errorf("commit download audit tx: %w", err)
	}
	return nil
}
