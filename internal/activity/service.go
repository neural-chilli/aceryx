package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
)

type Broadcaster interface {
	Broadcast(tenantID uuid.UUID, payload map[string]any) error
}

type FeedEvent struct {
	ID         uuid.UUID       `json:"id"`
	Type       string          `json:"type"`
	Text       string          `json:"text"`
	Icon       string          `json:"icon"`
	CaseID     uuid.UUID       `json:"case_id"`
	CaseNumber string          `json:"case_number"`
	ActorName  string          `json:"actor_name"`
	Timestamp  time.Time       `json:"timestamp"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

type Service struct {
	db  *sql.DB
	hub Broadcaster
}

func NewService(db *sql.DB, hub Broadcaster) *Service {
	return &Service{db: db, hub: hub}
}

var feedWorthy = map[string]map[string]struct{}{
	"case": {
		"created":   {},
		"closed":    {},
		"cancelled": {},
	},
	"task": {
		"claimed":   {},
		"completed": {},
		"escalated": {},
	},
	"agent": {
		"completed": {},
		"escalated": {},
	},
	"document": {
		"uploaded": {},
	},
	"system": {
		"sla_breach": {},
	},
}

func IsFeedWorthy(eventType, action string) bool {
	actions, ok := feedWorthy[eventType]
	if !ok {
		return false
	}
	_, ok = actions[action]
	return ok
}

func (s *Service) GetFeed(ctx context.Context, tenantID uuid.UUID, limit int, beforeTime *time.Time, beforeID *uuid.UUID) ([]FeedEvent, error) {
	return s.getFeed(ctx, tenantID, limit, beforeTime, beforeID, "all")
}

func (s *Service) GetFeedByFilter(ctx context.Context, tenantID uuid.UUID, limit int, beforeTime *time.Time, beforeID *uuid.UUID, filter string) ([]FeedEvent, error) {
	return s.getFeed(ctx, tenantID, limit, beforeTime, beforeID, filter)
}

func (s *Service) getFeed(ctx context.Context, tenantID uuid.UUID, limit int, beforeTime *time.Time, beforeID *uuid.UUID, filter string) ([]FeedEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	query := `
SELECT ce.id, ce.event_type, ce.action, COALESCE(ce.data, '{}'::jsonb), ce.created_at,
       ce.case_id, c.case_number, c.tenant_id,
       COALESCE(p.name, ''), COALESCE(ce.step_id, ''), COALESCE(cs.metadata->>'label', '')
FROM case_events ce
JOIN cases c ON c.id = ce.case_id
LEFT JOIN principals p ON p.id = ce.actor_id
LEFT JOIN case_steps cs ON cs.case_id = ce.case_id AND cs.step_id = ce.step_id
WHERE c.tenant_id = $1
  AND (ce.event_type, ce.action) IN (
    ('case','created'),
    ('case','closed'),
    ('case','cancelled'),
    ('task','claimed'),
    ('task','completed'),
    ('task','escalated'),
    ('agent','completed'),
    ('agent','escalated'),
    ('document','uploaded'),
    ('system','sla_breach')
  )
`
	args := []any{tenantID}
	if beforeTime != nil && beforeID != nil {
		args = append(args, *beforeTime, *beforeID)
		query += fmt.Sprintf(" AND (ce.created_at, ce.id) < ($%d, $%d)", len(args)-1, len(args))
	}

	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter != "" && filter != "all" {
		switch filter {
		case "cases":
			query += " AND ce.event_type = 'case'"
		case "tasks":
			query += " AND ce.event_type = 'task'"
		case "ai":
			query += " AND ce.event_type = 'agent'"
		case "documents":
			query += " AND ce.event_type = 'document'"
		case "sla":
			query += " AND ce.event_type = 'system' AND ce.action = 'sla_breach'"
		}
	}

	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY ce.created_at DESC, ce.id DESC LIMIT $%d", len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query activity feed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	terms, err := s.loadTerminology(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	out := make([]FeedEvent, 0, limit)
	for rows.Next() {
		var (
			eventType  string
			action     string
			data       []byte
			tenant     uuid.UUID
			stepID     string
			stepLabel  string
			actorName  string
			caseNumber string
			item       FeedEvent
		)
		if err := rows.Scan(&item.ID, &eventType, &action, &data, &item.Timestamp, &item.CaseID, &caseNumber, &tenant, &actorName, &stepID, &stepLabel); err != nil {
			return nil, fmt.Errorf("scan activity row: %w", err)
		}
		item.Type = eventType + "." + action
		item.CaseNumber = caseNumber
		item.ActorName = actorName
		item.Metadata = data
		item.Icon = iconFor(eventType)
		item.Text = formatEventText(eventType, action, actorName, caseNumber, stepID, stepLabel, data, terms)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate activity rows: %w", err)
	}
	return out, nil
}

func (s *Service) OnAuditEvent(event audit.Event) {
	if !IsFeedWorthy(event.EventType, event.Action) || s.hub == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var (
		tenantID   uuid.UUID
		caseNumber string
		actorName  string
		stepLabel  string
	)
	err := s.db.QueryRowContext(ctx, `
SELECT c.tenant_id, c.case_number, COALESCE(p.name, ''), COALESCE(cs.metadata->>'label', '')
FROM cases c
LEFT JOIN principals p ON p.id = $2
LEFT JOIN case_steps cs ON cs.case_id = c.id AND cs.step_id = NULLIF($3, '')
WHERE c.id = $1
`, event.CaseID, event.ActorID, event.StepID).Scan(&tenantID, &caseNumber, &actorName, &stepLabel)
	if err != nil {
		return
	}
	terms, err := s.loadTerminology(ctx, tenantID)
	if err != nil {
		return
	}
	feedEvent := FeedEvent{
		ID:         event.ID,
		Type:       event.EventType + "." + event.Action,
		Text:       formatEventText(event.EventType, event.Action, actorName, caseNumber, event.StepID, stepLabel, event.Data, terms),
		Icon:       iconFor(event.EventType),
		CaseID:     event.CaseID,
		CaseNumber: caseNumber,
		ActorName:  actorName,
		Timestamp:  event.CreatedAt,
		Metadata:   event.Data,
	}
	_ = s.hub.Broadcast(tenantID, map[string]any{
		"type":    "activity",
		"payload": feedEvent,
	})
}

func iconFor(eventType string) string {
	switch eventType {
	case "case":
		return "folder"
	case "task":
		return "check-square"
	case "agent":
		return "sparkles"
	case "document":
		return "file"
	case "system":
		return "alert-triangle"
	default:
		return "activity"
	}
}

func formatEventText(eventType, action, actorName, caseNumber, stepID, stepLabel string, metadata []byte, terms map[string]string) string {
	step := stepLabel
	if strings.TrimSpace(step) == "" {
		step = stepID
	}
	actor := strings.TrimSpace(actorName)
	if actor == "" {
		actor = "System"
	}
	switch eventType + "." + action {
	case "task.completed":
		outcome := ""
		var raw map[string]any
		_ = json.Unmarshal(metadata, &raw)
		if v, ok := raw["outcome"].(string); ok && strings.TrimSpace(v) != "" {
			outcome = strings.ToLower(strings.TrimSpace(v))
		}
		if outcome != "" {
			return fmt.Sprintf("%s %s %s", actor, outcome, caseNumber)
		}
		return fmt.Sprintf("%s completed %s %s", actor, term(terms, "task"), caseNumber)
	case "agent.completed":
		conf := ""
		var raw map[string]any
		_ = json.Unmarshal(metadata, &raw)
		if v, ok := raw["confidence"].(float64); ok {
			conf = fmt.Sprintf(" (confidence: %.2f)", v)
		}
		return fmt.Sprintf("AI completed %s on %s%s", fallback(step, "agent step"), caseNumber, conf)
	case "system.sla_breach":
		return fmt.Sprintf("SLA breached: %s on %s", fallback(step, "step"), caseNumber)
	case "case.created":
		return fmt.Sprintf("%s created %s %s", actor, term(terms, "case"), caseNumber)
	case "case.closed":
		return fmt.Sprintf("%s closed %s %s", actor, term(terms, "case"), caseNumber)
	case "case.cancelled":
		return fmt.Sprintf("%s cancelled %s %s", actor, term(terms, "case"), caseNumber)
	case "task.claimed":
		return fmt.Sprintf("%s claimed %s on %s", actor, term(terms, "task"), caseNumber)
	case "task.escalated":
		return fmt.Sprintf("%s escalated %s on %s", actor, fallback(step, term(terms, "task")), caseNumber)
	case "agent.escalated":
		return fmt.Sprintf("AI escalated %s on %s", fallback(step, "agent step"), caseNumber)
	case "document.uploaded":
		return fmt.Sprintf("%s uploaded a document to %s", actor, caseNumber)
	default:
		return fmt.Sprintf("%s %s.%s on %s", actor, eventType, action, caseNumber)
	}
}

func fallback(value, fb string) string {
	if strings.TrimSpace(value) == "" {
		return fb
	}
	return value
}

func term(terms map[string]string, key string) string {
	v := strings.TrimSpace(terms[key])
	if v == "" {
		return key
	}
	return v
}

func (s *Service) loadTerminology(ctx context.Context, tenantID uuid.UUID) (map[string]string, error) {
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT terminology FROM tenants WHERE id = $1`, tenantID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load terminology: %w", err)
	}
	out := map[string]string{
		"case":  "case",
		"cases": "cases",
		"task":  "task",
		"tasks": "tasks",
	}
	existing := map[string]string{}
	_ = json.Unmarshal(raw, &existing)
	for k, v := range existing {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	return out, nil
}
