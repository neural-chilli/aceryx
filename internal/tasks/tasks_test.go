package tasks

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSLAStatus(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	breached := now.Add(-time.Minute)
	warning := now.Add(10 * time.Minute)
	onTrack := now.Add(3 * time.Hour)
	started := now.Add(-time.Hour)

	if got := SLAStatus(now, &breached, &started, 2); got != "breached" {
		t.Fatalf("expected breached, got %s", got)
	}
	if got := SLAStatus(now, &warning, &started, 1); got != "warning" {
		t.Fatalf("expected warning, got %s", got)
	}
	if got := SLAStatus(now, &onTrack, &started, 4); got != "on_track" {
		t.Fatalf("expected on_track, got %s", got)
	}
}

func TestInboxOrdering(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	started := now.Add(-time.Hour)
	aDeadline := now.Add(-time.Minute)
	bDeadline := now.Add(10 * time.Minute)
	cDeadline := now.Add(30 * time.Minute)

	a := InboxTask{StepID: "a", Priority: 1, SLADeadline: &aDeadline, StartedAt: &started, SLAHours: 1}
	b := InboxTask{StepID: "b", Priority: 3, SLADeadline: &bDeadline, StartedAt: &started, SLAHours: 1}
	c := InboxTask{StepID: "c", Priority: 5, SLADeadline: &cDeadline, StartedAt: &started, SLAHours: 1}

	if !inboxLess(a, b, now) {
		t.Fatal("expected breached task before warning/on_track")
	}
	if !inboxLess(b, c, now) {
		t.Fatal("expected earlier deadline before later one")
	}
}

func TestSelectLeastLoaded(t *testing.T) {
	u1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	u2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	u3 := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	picked := selectLeastLoaded([]userLoad{
		{PrincipalID: u1, ActiveCount: 2, CreatedAt: now.Add(-2 * time.Hour)},
		{PrincipalID: u2, ActiveCount: 1, CreatedAt: now.Add(-1 * time.Hour)},
		{PrincipalID: u3, ActiveCount: 1, CreatedAt: now.Add(-3 * time.Hour)},
	})
	if picked != u3 {
		t.Fatalf("expected oldest among least-loaded users %s, got %s", u3, picked)
	}
}
