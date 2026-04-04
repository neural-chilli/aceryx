package integration

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/activity"
	"github.com/neural-chilli/aceryx/internal/audit"
)

type activityHubSpy struct {
	mu       sync.Mutex
	messages []map[string]any
}

func (s *activityHubSpy) Broadcast(_ uuid.UUID, payload map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := map[string]any{}
	for k, v := range payload {
		cp[k] = v
	}
	s.messages = append(s.messages, cp)
	return nil
}

func (s *activityHubSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func TestActivityFeed_OrderFilterTenantCursorAndCallbacks(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupPostgresWithMigrations(t)
	defer cleanup()

	tenantA, principalA := seedTenantAndPrincipal(t, ctx, db, "activity-a")
	tenantB, principalB := seedTenantAndPrincipal(t, ctx, db, "activity-b")
	caseA := seedAuditCase(t, ctx, db, tenantA, principalA, "activity_case_a")
	caseB := seedAuditCase(t, ctx, db, tenantB, principalB, "activity_case_b")

	if _, err := db.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, metadata, events)
VALUES ($1, 'underwriter_review', 'completed', '{"label":"Underwriter Review"}'::jsonb, '[]'::jsonb)
ON CONFLICT (case_id, step_id) DO UPDATE SET metadata = EXCLUDED.metadata
`, caseA); err != nil {
		t.Fatalf("seed case step metadata: %v", err)
	}

	hub := &activityHubSpy{}
	auditSvc := audit.NewService(db)
	activitySvc := activity.NewService(db, hub)
	auditSvc.OnCommitted(activitySvc.OnAuditEvent)

	id1 := appendAuditEventWithID(t, ctx, auditSvc, db, caseA, principalA, "case", "created", map[string]any{})
	id2 := appendAuditEventWithID(t, ctx, auditSvc, db, caseA, principalA, "task", "completed", map[string]any{"outcome": "approved"})
	_ = appendAuditEventWithID(t, ctx, auditSvc, db, caseA, principalA, "auth", "denied", map[string]any{"permission": "cases:update"})
	_ = appendAuditEventWithID(t, ctx, auditSvc, db, caseB, principalB, "case", "created", map[string]any{})

	feedA, err := activitySvc.GetFeed(ctx, tenantA, 50, nil, nil)
	if err != nil {
		t.Fatalf("get feed tenant A: %v", err)
	}
	if len(feedA) == 0 {
		t.Fatal("expected tenant A feed rows")
	}
	for _, item := range feedA {
		if item.CaseID == caseB {
			t.Fatal("tenant isolation failed: tenant B event leaked into tenant A feed")
		}
		if item.Type == "auth.denied" {
			t.Fatal("non-feed-worthy auth.denied should not be in feed")
		}
	}
	if feedA[0].Timestamp.Before(feedA[len(feedA)-1].Timestamp) {
		t.Fatal("feed not ordered by created_at desc")
	}

	// Force identical timestamps to validate compound cursor pagination by (created_at, id).
	same := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	if _, err := db.ExecContext(ctx, `UPDATE case_events SET created_at = $2 WHERE id IN ($1, $3)`, id1, same, id2); err != nil {
		t.Fatalf("set same timestamps: %v", err)
	}
	page1, err := activitySvc.GetFeed(ctx, tenantA, 1, nil, nil)
	if err != nil {
		t.Fatalf("page1 feed: %v", err)
	}
	if len(page1) != 1 {
		t.Fatalf("expected page1 len=1 got %d", len(page1))
	}
	beforeTime := page1[len(page1)-1].Timestamp
	beforeID := page1[len(page1)-1].ID
	page2, err := activitySvc.GetFeed(ctx, tenantA, 10, &beforeTime, &beforeID)
	if err != nil {
		t.Fatalf("page2 feed: %v", err)
	}
	if len(page2) == 0 {
		t.Fatal("expected second page with compound cursor")
	}
	seen := map[uuid.UUID]struct{}{}
	for _, item := range append(page1, page2...) {
		if _, ok := seen[item.ID]; ok {
			t.Fatalf("compound cursor duplicated event id %s", item.ID)
		}
		seen[item.ID] = struct{}{}
	}

	// Terminology-driven text.
	if _, err := db.ExecContext(ctx, `UPDATE tenants SET terminology='{"case":"application"}'::jsonb WHERE id = $1`, tenantA); err != nil {
		t.Fatalf("update terminology: %v", err)
	}
	_ = appendAuditEventWithID(t, ctx, auditSvc, db, caseA, principalA, "case", "created", map[string]any{})
	feedWithTerms, err := activitySvc.GetFeed(ctx, tenantA, 5, nil, nil)
	if err != nil {
		t.Fatalf("feed with terminology: %v", err)
	}
	foundTerm := false
	for _, item := range feedWithTerms {
		if item.Type == "case.created" && item.Text != "" && containsWord(item.Text, "application") {
			foundTerm = true
			break
		}
	}
	if !foundTerm {
		t.Fatal("expected terminology override to be applied in feed text")
	}

	// Post-commit callback fires only on commit and for feed-worthy events.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx rollback test: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1 FOR UPDATE`, caseA).Scan(new(uuid.UUID)); err != nil {
		t.Fatalf("lock case for rollback test: %v", err)
	}
	if err := auditSvc.RecordCaseEventTx(ctx, tx, caseA, "", "task", principalA, "human", "claimed", map[string]any{}); err != nil {
		t.Fatalf("record in rollback tx: %v", err)
	}
	if err := auditSvc.RollbackTx(tx); err != nil {
		t.Fatalf("rollback tx: %v", err)
	}
	beforeBroadcasts := hub.count()

	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx commit test: %v", err)
	}
	if err := tx2.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1 FOR UPDATE`, caseA).Scan(new(uuid.UUID)); err != nil {
		t.Fatalf("lock case for commit test: %v", err)
	}
	if err := auditSvc.RecordCaseEventTx(ctx, tx2, caseA, "", "task", principalA, "human", "claimed", map[string]any{}); err != nil {
		t.Fatalf("record in commit tx: %v", err)
	}
	if err := auditSvc.CommitTx(tx2); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	waitForCondition(t, 2*time.Second, 20*time.Millisecond, func() bool { return hub.count() > beforeBroadcasts }, "expected feed-worthy event broadcast")
	afterCommitBroadcasts := hub.count()

	tx3, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx non-feed-worthy: %v", err)
	}
	if err := tx3.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1 FOR UPDATE`, caseA).Scan(new(uuid.UUID)); err != nil {
		t.Fatalf("lock case for non-feed-worthy test: %v", err)
	}
	if err := auditSvc.RecordCaseEventTx(ctx, tx3, caseA, "", "auth", principalA, "human", "denied", map[string]any{}); err != nil {
		t.Fatalf("record non-feed-worthy event: %v", err)
	}
	if err := auditSvc.CommitTx(tx3); err != nil {
		t.Fatalf("commit non-feed-worthy tx: %v", err)
	}
	ensureConditionNever(t, 250*time.Millisecond, 20*time.Millisecond, func() bool {
		return hub.count() != afterCommitBroadcasts
	}, "non-feed-worthy event should not be broadcast")
	if hub.count() != afterCommitBroadcasts {
		t.Fatal("non-feed-worthy event should not be broadcast")
	}
}

func appendAuditEventWithID(t *testing.T, ctx context.Context, svc *audit.Service, db *sql.DB, caseID, actorID uuid.UUID, eventType, action string, data map[string]any) uuid.UUID {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM cases WHERE id = $1 FOR UPDATE`, caseID).Scan(new(uuid.UUID)); err != nil {
		_ = svc.RollbackTx(tx)
		t.Fatalf("lock case: %v", err)
	}
	if err := svc.RecordCaseEventTx(ctx, tx, caseID, "underwriter_review", eventType, actorID, "human", action, data); err != nil {
		_ = svc.RollbackTx(tx)
		t.Fatalf("record case event: %v", err)
	}
	if err := svc.CommitTx(tx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	var id uuid.UUID
	if err := db.QueryRowContext(ctx, `
SELECT id FROM case_events
WHERE case_id = $1 AND event_type = $2 AND action = $3
ORDER BY created_at DESC, id DESC
LIMIT 1
`, caseID, eventType, action).Scan(&id); err != nil {
		t.Fatalf("lookup recorded event id: %v", err)
	}
	return id
}

func containsWord(value, word string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(word))
}
