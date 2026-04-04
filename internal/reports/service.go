package reports

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/google/uuid"
)

const (
	defaultStatementTimeout = 5 * time.Second
	defaultRefreshInterval  = 5 * time.Minute
	defaultScheduleInterval = 1 * time.Minute
	maxRows                 = 10000
	maxResultBytes          = 5 * 1024 * 1024
)

type Service struct {
	db                *sql.DB
	reporterDB        *sql.DB
	llm               LLM
	inspector         SQLInspector
	logger            *log.Logger
	statementTimeout  time.Duration
	refreshInterval   time.Duration
	scheduleInterval  time.Duration
	now               func() time.Time
	sendScheduleEmail func(ctx context.Context, tenantID uuid.UUID, report SavedReport, rows []map[string]any, columns []string) error
}

func NewService(db *sql.DB, llm LLM) *Service {
	s := &Service{
		db:               db,
		reporterDB:       db,
		llm:              llm,
		inspector:        NewSQLInspector(),
		logger:           log.Default(),
		statementTimeout: defaultStatementTimeout,
		refreshInterval:  defaultRefreshInterval,
		scheduleInterval: defaultScheduleInterval,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	s.sendScheduleEmail = s.defaultScheduleEmail
	return s
}

func (s *Service) SetScheduleEmailSender(fn func(ctx context.Context, tenantID uuid.UUID, report SavedReport, rows []map[string]any, columns []string) error) {
	if fn == nil {
		s.sendScheduleEmail = s.defaultScheduleEmail
		return
	}
	s.sendScheduleEmail = fn
}
