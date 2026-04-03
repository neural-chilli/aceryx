package vault

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
)

type Service struct {
	db              *sql.DB
	store           VaultStore
	cleanupInterval time.Duration
	now             func() time.Time
	systemActorID   uuid.UUID
}

type UploadInput struct {
	CaseID     uuid.UUID
	Filename   string
	MimeType   string
	Data       []byte
	Metadata   map[string]any
	StepID     string
	UploadedBy uuid.UUID
}

type Document struct {
	ID          uuid.UUID       `json:"id"`
	CaseID      uuid.UUID       `json:"case_id"`
	StepID      string          `json:"step_id,omitempty"`
	Filename    string          `json:"filename"`
	MimeType    string          `json:"mime_type"`
	SizeBytes   int64           `json:"size_bytes"`
	ContentHash string          `json:"content_hash"`
	UploadedBy  uuid.UUID       `json:"uploaded_by"`
	UploadedAt  time.Time       `json:"uploaded_at"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	DisplayMode string          `json:"display_mode"`
}

type ErasureRequest struct {
	CaseID           *uuid.UUID `json:"case_id,omitempty"`
	DataSubjectEmail string     `json:"data_subject_email,omitempty"`
}

type SignedDocumentURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewService(db *sql.DB, store VaultStore, cleanupInterval time.Duration) *Service {
	if cleanupInterval <= 0 {
		cleanupInterval = 24 * time.Hour
	}
	return &Service{db: db, store: store, cleanupInterval: cleanupInterval, now: func() time.Time { return time.Now().UTC() }, systemActorID: uuid.Nil}
}

func (s *Service) SetSystemActorID(actorID uuid.UUID) {
	s.systemActorID = actorID
}

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
	defer func() { _ = audit.RollbackTx(tx) }()

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
	if err := audit.RecordCaseEventTx(ctx, tx, in.CaseID, in.StepID, "document", in.UploadedBy, "human", "uploaded", map[string]any{"document_id": doc.ID.String(), "filename": in.Filename, "size_bytes": doc.SizeBytes, "content_hash": hash}); err != nil {
		return Document{}, err
	}
	if err := audit.CommitTx(tx); err != nil {
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
	defer func() { _ = audit.RollbackTx(tx) }()

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
	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "document", actorID, "human", "deleted", map[string]any{"document_id": docID.String(), "filename": filename}); err != nil {
		return err
	}
	if err := audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit delete document tx: %w", err)
	}
	return nil
}

func (s *Service) SignedURL(ctx context.Context, tenantID, caseID, docID uuid.UUID, expirySeconds int) (SignedDocumentURL, error) {
	_, storageURI, err := s.getDocumentRecord(ctx, tenantID, caseID, docID)
	if err != nil {
		return SignedDocumentURL{}, err
	}
	signedPath, err := s.store.SignedURL(storageURI, expirySeconds)
	if err != nil {
		return SignedDocumentURL{}, err
	}
	parts := strings.SplitN(signedPath, "?", 2)
	if len(parts) != 2 {
		return SignedDocumentURL{}, fmt.Errorf("invalid signed url output")
	}
	expValue := ""
	for _, kv := range strings.Split(parts[1], "&") {
		if strings.HasPrefix(kv, "exp=") {
			expValue = strings.TrimPrefix(kv, "exp=")
		}
	}
	expUnix, _ := strconv.ParseInt(expValue, 10, 64)
	url := fmt.Sprintf("/vault/signed/%s?case_id=%s&uri=%s&%s", docID, caseID, url.QueryEscape(parts[0]), parts[1])
	return SignedDocumentURL{URL: url, ExpiresAt: time.Unix(expUnix, 0).UTC()}, nil
}

func (s *Service) DownloadFromSignedURL(ctx context.Context, docID uuid.UUID, caseID uuid.UUID, uri, expiry, signature string) (Document, []byte, error) {
	lv, ok := s.store.(*LocalVaultStore)
	if !ok {
		return Document{}, nil, fmt.Errorf("signed url validation is unavailable for configured vault store")
	}
	if err := lv.VerifySignedURL(uri, expiry, signature); err != nil {
		return Document{}, nil, err
	}

	var (
		tenantID uuid.UUID
		doc      Document
	)
	err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, case_id, COALESCE(step_id,''), filename, mime_type, size_bytes, content_hash, uploaded_by, uploaded_at, COALESCE(metadata, '{}'::jsonb)
FROM vault_documents
WHERE id = $1 AND case_id = $2 AND deleted_at IS NULL
`, docID, caseID).Scan(&doc.ID, &tenantID, &doc.CaseID, &doc.StepID, &doc.Filename, &doc.MimeType, &doc.SizeBytes, &doc.ContentHash, &doc.UploadedBy, &doc.UploadedAt, &doc.Metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Document{}, nil, sql.ErrNoRows
		}
		return Document{}, nil, fmt.Errorf("load document for signed download: %w", err)
	}
	if !strings.HasPrefix(uri, tenantID.String()+"/") {
		return Document{}, nil, fmt.Errorf("signed url tenant mismatch")
	}
	blob, err := s.store.Get(uri)
	if err != nil {
		return Document{}, nil, err
	}
	if err := s.recordDownloadAudit(ctx, doc.CaseID, doc.StepID, s.systemActorID, doc.ID, doc.Filename); err != nil {
		return Document{}, nil, err
	}
	doc.DisplayMode = DisplayModeForMime(doc.MimeType)
	return doc, blob, nil
}

func (s *Service) OrphanCleanup(ctx context.Context, tenantID *uuid.UUID) (int, int64, error) {
	query := `
SELECT DISTINCT vd1.tenant_id, vd1.content_hash, vd1.storage_uri
FROM vault_documents vd1
WHERE vd1.deleted_at IS NOT NULL
`
	args := []any{}
	if tenantID != nil {
		query += ` AND vd1.tenant_id = $1`
		args = append(args, *tenantID)
	}
	if tenantID != nil {
		query += ` AND NOT EXISTS (
  SELECT 1 FROM vault_documents vd2
  WHERE vd2.content_hash = vd1.content_hash
    AND vd2.tenant_id = vd1.tenant_id
    AND vd2.deleted_at IS NULL
)`
	} else {
		query += ` AND NOT EXISTS (
  SELECT 1 FROM vault_documents vd2
  WHERE vd2.content_hash = vd1.content_hash
    AND vd2.tenant_id = vd1.tenant_id
    AND vd2.deleted_at IS NULL
)`
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *Service) StartOrphanCleanupTicker(ctx context.Context) {
	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_, _, _ = s.OrphanCleanup(ctx, nil)
		case <-ctx.Done():
			return
		}
	}
}

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
	defer func() { _ = audit.RollbackTx(tx) }()

	for _, caseID := range caseIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE vault_documents SET deleted_at = now() WHERE tenant_id = $1 AND case_id = $2 AND deleted_at IS NULL`, tenantID, caseID); err != nil {
			return fmt.Errorf("logical delete documents for erasure: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE case_steps SET events = '[]'::jsonb WHERE case_id = $1`, caseID); err != nil {
			return fmt.Errorf("purge case step events for erasure: %w", err)
		}
		if err := audit.RecordCaseEventTx(ctx, tx, caseID, "", "system", actorID, "human", "erasure_completed", map[string]any{"case_id": caseID.String(), "data_subject_email": req.DataSubjectEmail}); err != nil {
			return err
		}
	}
	if err := audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit erasure tx: %w", err)
	}
	for _, caseID := range caseIDs {
		_ = caseID
		_, _, _ = s.OrphanCleanup(ctx, &tenantID)
	}
	return nil
}

func (s *Service) resolveErasureCases(ctx context.Context, tenantID uuid.UUID, req ErasureRequest) ([]uuid.UUID, error) {
	if req.CaseID != nil && *req.CaseID != uuid.Nil {
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
	defer func() { _ = audit.RollbackTx(tx) }()
	if err := audit.RecordCaseEventTx(ctx, tx, caseID, stepID, "document", actorID, "human", "downloaded", map[string]any{"document_id": docID.String(), "filename": filename}); err != nil {
		return err
	}
	if err := audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit download audit tx: %w", err)
	}
	return nil
}

func DisplayModeForMime(m string) string {
	m = strings.ToLower(strings.TrimSpace(strings.Split(m, ";")[0]))
	switch m {
	case "application/pdf", "image/png", "image/jpeg", "image/gif", "image/webp", "text/plain", "text/markdown", "text/csv":
		return "inline"
	default:
		return "download"
	}
}

func detectMime(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return "application/octet-stream"
	}
	if m := mime.TypeByExtension(ext); m != "" {
		return m
	}
	return "application/octet-stream"
}

func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
