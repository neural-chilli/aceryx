package ai

import (
	"encoding/json"
	"testing"
)

func TestUpsertRequestJSON(t *testing.T) {
	payload := UpsertRequest{Definition: &AIComponentDef{ID: "x", DisplayLabel: "X", Category: "AI", Tier: TierCommercial, InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`), SystemPrompt: "s", UserPromptTmpl: "u"}}
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("marshal upsert request: %v", err)
	}
}
