package cases

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
	"github.com/neural-chilli/aceryx/internal/notify"
)

type Engine interface {
	EvaluateDAG(ctx context.Context, caseID uuid.UUID) error
	CancelCase(ctx context.Context, caseID uuid.UUID, actorID uuid.UUID, reason string) error
}

type CaseTypeService struct {
	db *sql.DB
}

type CaseService struct {
	db     *sql.DB
	engine Engine
	notify Notifier
	audit  *audit.Service
}

type Notifier interface {
	Notify(ctx context.Context, event notify.NotifyEvent) error
}

type ReportsService struct {
	db              *sql.DB
	refreshInterval time.Duration
}

func NewCaseTypeService(db *sql.DB) *CaseTypeService {
	return &CaseTypeService{db: db}
}

func NewCaseService(db *sql.DB, eng Engine) *CaseService {
	return &CaseService{db: db, engine: eng, audit: audit.NewService(db)}
}

func (s *CaseService) SetNotifier(n Notifier) {
	s.notify = n
}

func (s *CaseService) SetAuditService(auditSvc *audit.Service) {
	if auditSvc == nil {
		return
	}
	s.audit = auditSvc
}

func NewReportsService(db *sql.DB, refreshInterval time.Duration) *ReportsService {
	if refreshInterval <= 0 {
		refreshInterval = 5 * time.Minute
	}
	return &ReportsService{db: db, refreshInterval: refreshInterval}
}
