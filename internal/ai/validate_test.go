package ai

import (
	"encoding/json"
	"testing"
)

func TestValidateOutputPassesForValidOutput(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["ok","bad"]},"score":{"type":"number","minimum":0,"maximum":1}},"required":["status","score"]}`)
	output := json.RawMessage(`{"status":"ok","score":0.8}`)
	errList := ValidateOutput(output, schema)
	if len(errList) != 0 {
		t.Fatalf("expected no errors, got %#v", errList)
	}
}

func TestValidateOutputMissingRequiredFails(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"status":{"type":"string"}},"required":["status"]}`)
	output := json.RawMessage(`{}`)
	errList := ValidateOutput(output, schema)
	if len(errList) == 0 {
		t.Fatalf("expected validation errors")
	}
}

func TestValidateOutputWrongTypeAndEnumFails(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["open","closed"]},"count":{"type":"number"}},"required":["status","count"]}`)
	output := json.RawMessage(`{"status":"pending","count":"many"}`)
	errList := ValidateOutput(output, schema)
	if len(errList) < 2 {
		t.Fatalf("expected >=2 errors, got %#v", errList)
	}
}
