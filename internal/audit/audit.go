package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID            uuid.UUID       `json:"id"`
	CaseID        uuid.UUID       `json:"case_id"`
	StepID        string          `json:"step_id,omitempty"`
	EventType     string          `json:"event_type"`
	ActorID       uuid.UUID       `json:"actor_id"`
	ActorType     string          `json:"actor_type"`
	Action        string          `json:"action"`
	Data          json.RawMessage `json:"data,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	PrevEventHash string          `json:"prev_event_hash"`
	EventHash     string          `json:"event_hash"`
}

type ListFilter struct {
	EventType string
	Action    string
	Page      int
	PerPage   int
}

type VerifyResult struct {
	Valid         bool       `json:"valid"`
	EventCount    int        `json:"event_count"`
	BrokenAtIndex int        `json:"broken_at_index,omitempty"`
	BrokenEventID *uuid.UUID `json:"broken_event_id,omitempty"`
	ExpectedHash  string     `json:"expected_hash,omitempty"`
	ActualHash    string     `json:"actual_hash,omitempty"`
	FirstEvent    *time.Time `json:"first_event,omitempty"`
	LastEvent     *time.Time `json:"last_event,omitempty"`
}

type Service struct {
	db *sql.DB

	mu        sync.RWMutex
	callbacks []func(Event)
	pending   map[*sql.Tx][]Event
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, pending: make(map[*sql.Tx][]Event)}
}

// Registry of known event_type/action combinations.
var EventTypeRegistry = map[string]map[string]struct{}{
	"case":     {"created": {}, "updated": {}, "closed": {}, "cancelled": {}},
	"step":     {"activated": {}, "completed": {}, "failed": {}, "skipped": {}},
	"task":     {"created": {}, "claimed": {}, "completed": {}, "reassigned": {}, "escalated": {}, "escalation_suppressed": {}},
	"agent":    {"completed": {}, "escalated": {}},
	"document": {"uploaded": {}, "downloaded": {}, "deleted": {}},
	"auth":     {"denied": {}, "login": {}, "logout": {}, "password_changed": {}},
	"system":   {"sla_breach": {}, "workflow_published": {}, "erasure_completed": {}},
}

func IsRegistered(eventType, action string) bool {
	actions, ok := EventTypeRegistry[eventType]
	if !ok {
		return false
	}
	_, ok = actions[action]
	return ok
}

func GenesisHash(caseID uuid.UUID) string {
	sum := sha256.Sum256([]byte("aceryx:genesis:" + caseID.String()))
	return hex.EncodeToString(sum[:])
}

func ComputeHash(prev, eventType, actorID, action string, data json.RawMessage, createdAt time.Time) string {
	payload := prev + eventType + actorID + action + canonicalJSON(data) + createdAt.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func canonicalJSON(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return string(data)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return string(data)
	}
	return string(canonical)
}

func VerifyChain(events []Event) (bool, int, error) {
	if len(events) == 0 {
		return true, -1, nil
	}
	caseID := events[0].CaseID
	prev := GenesisHash(caseID)
	for i, event := range events {
		if event.CaseID != caseID {
			return false, i, fmt.Errorf("mixed case ids in event chain")
		}
		if i > 0 && event.CreatedAt.Before(events[i-1].CreatedAt) {
			return false, i, nil
		}
		if event.PrevEventHash != prev {
			return false, i, nil
		}
		expected := ComputeHash(prev, event.EventType, event.ActorID.String(), event.Action, event.Data, event.CreatedAt)
		if expected != event.EventHash {
			return false, i, nil
		}
		prev = event.EventHash
	}
	return true, -1, nil
}

func (s *Service) OnCommitted(fn func(Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, fn)
}

func (s *Service) recordPending(tx *sql.Tx, event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[tx] = append(s.pending[tx], event)
}

func (s *Service) flushPending(tx *sql.Tx) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := append([]Event(nil), s.pending[tx]...)
	delete(s.pending, tx)
	return events
}

func (s *Service) callbacksSnapshot() []func(Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]func(Event){}, s.callbacks...)
}

func (s *Service) CommitTx(tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("tx is nil")
	}
	if err := tx.Commit(); err != nil {
		_ = s.flushPending(tx)
		return err
	}
	events := s.flushPending(tx)
	callbacks := s.callbacksSnapshot()
	for _, event := range events {
		ev := event
		for _, cb := range callbacks {
			callback := cb
			go func() {
				defer func() { _ = recover() }()
				callback(ev)
			}()
		}
	}
	return nil
}

func (s *Service) RollbackTx(tx *sql.Tx) error {
	if tx == nil {
		return nil
	}
	_ = s.flushPending(tx)
	if err := tx.Rollback(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") {
		return err
	}
	return nil
}

func (s *Service) RecordCaseEventTx(
	ctx context.Context,
	tx *sql.Tx,
	caseID uuid.UUID,
	stepID string,
	eventType string,
	actorID uuid.UUID,
	actorType string,
	action string,
	data map[string]any,
) error {
	if !IsRegistered(eventType, action) {
		return fmt.Errorf("unregistered audit event %s.%s", eventType, action)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, caseID.String()); err != nil {
		return fmt.Errorf("acquire audit chain lock: %w", err)
	}
	var (
		prev        string
		prevCreated time.Time
	)
	err := tx.QueryRowContext(ctx, `
SELECT event_hash, created_at
FROM case_events
WHERE case_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1
`, caseID).Scan(&prev, &prevCreated)
	if err != nil {
		if err == sql.ErrNoRows {
			prev = GenesisHash(caseID)
		} else {
			return fmt.Errorf("load previous event hash: %w", err)
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if !prevCreated.IsZero() && !now.After(prevCreated) {
		now = prevCreated.Add(time.Microsecond)
	}
	hash := ComputeHash(prev, eventType, actorID.String(), action, raw, now)

	var eventID uuid.UUID
	if err := tx.QueryRowContext(ctx, `
INSERT INTO case_events (
    case_id, step_id, event_type, actor_id, actor_type, action, data, created_at, prev_event_hash, event_hash
) VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7::jsonb, $8, $9, $10)
RETURNING id
`, caseID, stepID, eventType, actorID, actorType, action, string(raw), now, prev, hash).Scan(&eventID); err != nil {
		return fmt.Errorf("insert case event: %w", err)
	}
	s.recordPending(tx, Event{
		ID:            eventID,
		CaseID:        caseID,
		StepID:        stepID,
		EventType:     eventType,
		ActorID:       actorID,
		ActorType:     actorType,
		Action:        action,
		Data:          raw,
		CreatedAt:     now,
		PrevEventHash: prev,
		EventHash:     hash,
	})
	return nil
}

func (s *Service) ListCaseEvents(ctx context.Context, caseID uuid.UUID, filter ListFilter) ([]Event, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PerPage <= 0 {
		filter.PerPage = 50
	}
	if filter.PerPage > 200 {
		filter.PerPage = 200
	}
	offset := (filter.Page - 1) * filter.PerPage

	query := `
SELECT id, case_id, COALESCE(step_id, ''), event_type, actor_id, actor_type, action, COALESCE(data, '{}'::jsonb), created_at, prev_event_hash, event_hash
FROM case_events
WHERE case_id = $1
`
	args := []any{caseID}
	if filter.EventType != "" {
		args = append(args, filter.EventType)
		query += fmt.Sprintf(" AND event_type = $%d", len(args))
	}
	if filter.Action != "" {
		args = append(args, filter.Action)
		query += fmt.Sprintf(" AND action = $%d", len(args))
	}
	args = append(args, filter.PerPage, offset)
	query += fmt.Sprintf(" ORDER BY created_at, id LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query case events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Event, 0)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.CaseID, &event.StepID, &event.EventType, &event.ActorID, &event.ActorType, &event.Action, &event.Data, &event.CreatedAt, &event.PrevEventHash, &event.EventHash); err != nil {
			return nil, fmt.Errorf("scan case event: %w", err)
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Service) CaseInTenant(ctx context.Context, caseID, tenantID uuid.UUID) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM cases
    WHERE id = $1 AND tenant_id = $2
)
`, caseID, tenantID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check case tenant scope: %w", err)
	}
	return exists, nil
}

func (s *Service) VerifyCaseChain(ctx context.Context, caseID uuid.UUID) (VerifyResult, error) {
	events, err := s.ListCaseEvents(ctx, caseID, ListFilter{Page: 1, PerPage: 1000000})
	if err != nil {
		return VerifyResult{}, err
	}
	result := VerifyResult{EventCount: len(events), Valid: true}
	if len(events) > 0 {
		first := events[0].CreatedAt
		last := events[len(events)-1].CreatedAt
		result.FirstEvent = &first
		result.LastEvent = &last
	}
	valid, brokenAt, err := VerifyChain(events)
	if err != nil {
		return VerifyResult{}, err
	}
	result.Valid = valid
	if !valid && brokenAt >= 0 && brokenAt < len(events) {
		result.BrokenAtIndex = brokenAt
		id := events[brokenAt].ID
		result.BrokenEventID = &id
		prev := GenesisHash(caseID)
		if brokenAt > 0 {
			prev = events[brokenAt-1].EventHash
		}
		result.ExpectedHash = ComputeHash(prev, events[brokenAt].EventType, events[brokenAt].ActorID.String(), events[brokenAt].Action, events[brokenAt].Data, events[brokenAt].CreatedAt)
		result.ActualHash = events[brokenAt].EventHash
	}
	return result, nil
}

func (s *Service) ExportCaseEventsJSON(ctx context.Context, caseID uuid.UUID) ([]byte, error) {
	events, err := s.ListCaseEvents(ctx, caseID, ListFilter{Page: 1, PerPage: 1000000})
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(events)
	if err != nil {
		return nil, fmt.Errorf("marshal json export: %w", err)
	}
	return out, nil
}

func (s *Service) ExportCaseEventsCSV(ctx context.Context, caseID uuid.UUID) ([]byte, error) {
	events, err := s.ListCaseEvents(ctx, caseID, ListFilter{Page: 1, PerPage: 1000000})
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	writer := csv.NewWriter(&b)
	if err := writer.Write([]string{"id", "case_id", "step_id", "event_type", "actor_id", "actor_type", "action", "data", "created_at", "prev_event_hash", "event_hash"}); err != nil {
		return nil, err
	}
	for _, event := range events {
		if err := writer.Write([]string{
			event.ID.String(),
			event.CaseID.String(),
			event.StepID,
			event.EventType,
			event.ActorID.String(),
			event.ActorType,
			event.Action,
			string(event.Data),
			event.CreatedAt.UTC().Format(time.RFC3339Nano),
			event.PrevEventHash,
			event.EventHash,
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

var defaultSvcMu sync.RWMutex
var defaultSvc *Service

func SetDefaultService(svc *Service) {
	defaultSvcMu.Lock()
	defer defaultSvcMu.Unlock()
	defaultSvc = svc
}

func getDefaultService() *Service {
	defaultSvcMu.RLock()
	defer defaultSvcMu.RUnlock()
	return defaultSvc
}

// RecordCaseEventTx is the package-level compatibility helper used by existing packages.
func RecordCaseEventTx(
	ctx context.Context,
	tx *sql.Tx,
	caseID uuid.UUID,
	stepID string,
	eventType string,
	actorID uuid.UUID,
	actorType string,
	action string,
	data map[string]any,
) error {
	svc := getDefaultService()
	if svc == nil {
		svc = NewService(nil)
	}
	return svc.RecordCaseEventTx(ctx, tx, caseID, stepID, eventType, actorID, actorType, action, data)
}

func CommitTx(tx *sql.Tx) error {
	svc := getDefaultService()
	if svc == nil {
		return tx.Commit()
	}
	return svc.CommitTx(tx)
}

func RollbackTx(tx *sql.Tx) error {
	svc := getDefaultService()
	if svc == nil {
		if tx == nil {
			return nil
		}
		return tx.Rollback()
	}
	return svc.RollbackTx(tx)
}
