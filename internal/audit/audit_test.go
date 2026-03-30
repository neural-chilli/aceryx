package audit

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenesisHashDeterministic(t *testing.T) {
	caseID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	first := GenesisHash(caseID)
	second := GenesisHash(caseID)
	if first != second {
		t.Fatalf("expected deterministic genesis hash, got %q and %q", first, second)
	}
}

func TestGenesisHashDiffersPerCase(t *testing.T) {
	a := GenesisHash(uuid.MustParse("11111111-1111-1111-1111-111111111111"))
	b := GenesisHash(uuid.MustParse("22222222-2222-2222-2222-222222222222"))
	if a == b {
		t.Fatal("expected different genesis hashes for different cases")
	}
}

func TestComputeHashDeterministic(t *testing.T) {
	createdAt := time.Date(2026, time.March, 30, 12, 0, 0, 0, time.UTC)
	data := json.RawMessage(`{"x":1}`)
	first := ComputeHash("prev", "case", uuid.Nil.String(), "created", data, createdAt)
	second := ComputeHash("prev", "case", uuid.Nil.String(), "created", data, createdAt)
	if first != second {
		t.Fatalf("expected deterministic event hash, got %q and %q", first, second)
	}
}

func TestComputeHashChangesWhenInputChanges(t *testing.T) {
	createdAt := time.Date(2026, time.March, 30, 12, 0, 0, 0, time.UTC)
	data := json.RawMessage(`{"x":1}`)
	base := ComputeHash("prev", "case", uuid.Nil.String(), "created", data, createdAt)
	changed := ComputeHash("prev", "case", uuid.Nil.String(), "updated", data, createdAt)
	if base == changed {
		t.Fatal("expected hash to change when action changes")
	}
}

func TestVerifyChainValid(t *testing.T) {
	events := buildValidChain(t, 3)
	valid, brokenAt, err := VerifyChain(events)
	if err != nil {
		t.Fatalf("verify chain returned error: %v", err)
	}
	if !valid || brokenAt != -1 {
		t.Fatalf("expected valid chain, got valid=%v brokenAt=%d", valid, brokenAt)
	}
}

func TestVerifyChainTamperedData(t *testing.T) {
	events := buildValidChain(t, 3)
	events[1].Data = json.RawMessage(`{"tampered":true}`)
	valid, brokenAt, err := VerifyChain(events)
	if err != nil {
		t.Fatalf("verify chain returned error: %v", err)
	}
	if valid {
		t.Fatal("expected invalid chain after tampering")
	}
	if brokenAt != 1 {
		t.Fatalf("expected break at index 1, got %d", brokenAt)
	}
}

func TestVerifyChainDeletedEventGap(t *testing.T) {
	events := buildValidChain(t, 4)
	withGap := []Event{events[0], events[2], events[3]}
	valid, brokenAt, err := VerifyChain(withGap)
	if err != nil {
		t.Fatalf("verify chain returned error: %v", err)
	}
	if valid {
		t.Fatal("expected invalid chain for deleted event gap")
	}
	if brokenAt != 1 {
		t.Fatalf("expected break at index 1, got %d", brokenAt)
	}
}

func TestVerifyChainReordered(t *testing.T) {
	events := buildValidChain(t, 3)
	reordered := []Event{events[0], events[2], events[1]}
	valid, brokenAt, err := VerifyChain(reordered)
	if err != nil {
		t.Fatalf("verify chain returned error: %v", err)
	}
	if valid {
		t.Fatal("expected invalid chain for reordered events")
	}
	if brokenAt != 1 {
		t.Fatalf("expected break at index 1, got %d", brokenAt)
	}
}

func buildValidChain(t *testing.T, n int) []Event {
	t.Helper()
	caseID := uuid.New()
	actorID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	prev := GenesisHash(caseID)
	baseTime := time.Date(2026, time.March, 30, 12, 0, 0, 0, time.UTC)
	events := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		createdAt := baseTime.Add(time.Duration(i) * time.Second)
		data := json.RawMessage(fmt.Sprintf(`{"idx":%d}`, i))
		hash := ComputeHash(prev, "step", actorID.String(), "completed", data, createdAt)
		events = append(events, Event{
			ID:            uuid.New(),
			CaseID:        caseID,
			EventType:     "step",
			ActorID:       actorID,
			ActorType:     "system",
			Action:        "completed",
			Data:          data,
			CreatedAt:     createdAt,
			PrevEventHash: prev,
			EventHash:     hash,
		})
		prev = hash
	}
	return events
}
