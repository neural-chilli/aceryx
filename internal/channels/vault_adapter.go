package channels

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/vault"
)

type VaultAttachmentService struct {
	db    *sql.DB
	vault *vault.Service
}

func NewVaultAttachmentService(db *sql.DB, svc *vault.Service) *VaultAttachmentService {
	return &VaultAttachmentService{db: db, vault: svc}
}

func (s *VaultAttachmentService) Store(ctx context.Context, tenantID, caseID uuid.UUID, in AttachmentInput) (AttachmentRef, error) {
	if s == nil || s.vault == nil {
		return AttachmentRef{}, fmt.Errorf("vault service not configured")
	}
	actorID, err := s.resolveActor(ctx, tenantID, caseID)
	if err != nil {
		return AttachmentRef{}, err
	}
	mime := strings.TrimSpace(in.ContentType)
	if mime == "" {
		mime = http.DetectContentType(in.Data)
	}
	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		filename = "attachment" + extForMime(mime)
	}
	doc, err := s.vault.Upload(ctx, tenantID, vault.UploadInput{
		CaseID:     caseID,
		Filename:   filename,
		MimeType:   mime,
		Data:       in.Data,
		Metadata:   map[string]any{"source": "channel"},
		UploadedBy: actorID,
	})
	if err != nil {
		return AttachmentRef{}, err
	}
	sum := sha256.Sum256(in.Data)
	return AttachmentRef{
		VaultID:     doc.ID,
		Filename:    doc.Filename,
		ContentType: doc.MimeType,
		Size:        doc.SizeBytes,
		Checksum:    hex.EncodeToString(sum[:]),
	}, nil
}

func (s *VaultAttachmentService) resolveActor(ctx context.Context, tenantID, caseID uuid.UUID) (uuid.UUID, error) {
	if s == nil || s.db == nil {
		return uuid.Nil, fmt.Errorf("db not configured")
	}
	var actorID uuid.UUID
	if err := s.db.QueryRowContext(ctx, `
SELECT created_by
FROM cases
WHERE tenant_id = $1 AND id = $2
`, tenantID, caseID).Scan(&actorID); err == nil {
		return actorID, nil
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT id
FROM principals
WHERE tenant_id = $1 AND status = 'active'
ORDER BY created_at ASC
LIMIT 1
`, tenantID).Scan(&actorID); err != nil {
		return uuid.Nil, fmt.Errorf("resolve actor for attachment upload: %w", err)
	}
	return actorID, nil
}

func extForMime(mime string) string {
	switch mime {
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "text/csv":
		return ".csv"
	default:
		ext := filepath.Ext(mime)
		if ext != "" {
			return ext
		}
		return ".bin"
	}
}
