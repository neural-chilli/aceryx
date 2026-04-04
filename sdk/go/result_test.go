package sdk

import (
	"encoding/json"
	"testing"
)

func TestResultHelpers(t *testing.T) {
	if got := OK(); got.Status != "ok" {
		t.Fatalf("OK status = %s", got.Status)
	}

	out := OKWithOutput(map[string]any{"x": 1})
	if out.Status != "ok" {
		t.Fatalf("OKWithOutput status = %s", out.Status)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Output, &decoded); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if decoded["x"].(float64) != 1 {
		t.Fatalf("unexpected output value: %v", decoded["x"])
	}

	errResult := Error("boom")
	if errResult.Status != "error" || errResult.Error != "boom" {
		t.Fatalf("unexpected error result: %+v", errResult)
	}

	withCode := ErrorWithCode("E_TEST", "bad")
	if withCode.Code != "E_TEST" || withCode.Error != "bad" {
		t.Fatalf("unexpected error code result: %+v", withCode)
	}
}
