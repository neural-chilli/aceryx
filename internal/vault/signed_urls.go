package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Service) SignedURL(ctx context.Context, tenantID, caseID, docID uuid.UUID, expirySeconds int) (SignedDocumentURL, error) {
	_, storageURI, err := s.getDocumentRecord(ctx, tenantID, caseID, docID)
	if err != nil {
		return SignedDocumentURL{}, err
	}
	signedPath, err := s.store.SignedURL(storageURI, expirySeconds)
	if err != nil {
		return SignedDocumentURL{}, err
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(signedPath)), "http://") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(signedPath)), "https://") {
		expiresAt := s.now().Add(15 * time.Minute).UTC()
		if expirySeconds > 0 {
			expiresAt = s.now().Add(time.Duration(expirySeconds) * time.Second).UTC()
		}
		return SignedDocumentURL{URL: signedPath, ExpiresAt: expiresAt}, nil
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
