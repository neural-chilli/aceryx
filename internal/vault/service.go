package vault

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
)

type Service struct {
	db              *sql.DB
	store           VaultStore
	auditSvc        *audit.Service
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
	return &Service{
		db:              db,
		store:           store,
		auditSvc:        audit.NewService(db),
		cleanupInterval: cleanupInterval,
		now:             func() time.Time { return time.Now().UTC() },
		systemActorID:   uuid.Nil,
	}
}

func (s *Service) SetSystemActorID(actorID uuid.UUID) {
	s.systemActorID = actorID
}

func (s *Service) SetAuditService(auditSvc *audit.Service) {
	if auditSvc == nil {
		return
	}
	s.auditSvc = auditSvc
}
