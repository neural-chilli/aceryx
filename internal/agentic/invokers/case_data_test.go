package invokers

import (
	"context"
	"encoding/json"
	"testing"
)

type memCaseStore struct {
	data map[string]any
}

func (m *memCaseStore) GetCaseData(context.Context) (map[string]any, error) { return m.data, nil }
func (m *memCaseStore) MergeCaseData(_ context.Context, patch map[string]any) error {
	m.data = patch
	return nil
}

func TestCaseDataInvoker_ReadAndWrite(t *testing.T) {
	store := &memCaseStore{data: map[string]any{"applicant": map[string]any{"name": "A"}}}
	inv := NewCaseDataInvoker(store, false)
	read, err := inv.Invoke(context.Background(), json.RawMessage(`{"path":"applicant.name"}`))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(read) == "" {
		t.Fatalf("expected read payload")
	}
	if _, err := inv.Invoke(context.Background(), json.RawMessage(`{"path":"computed.score","value":0.8}`)); err != nil {
		t.Fatalf("write error: %v", err)
	}
}
