package sdk

import (
	"testing"
	"time"
)

func TestPollHTTPCallsHTTPAndSleeps(t *testing.T) {
	mock := NewMockContext()
	mock.SetHTTPResponse("https://poll.example.com", Response{Status: 200, Body: []byte("ok")})

	ctx := newTriggerContextWithBridge(mock)

	start := time.Now()
	resp, err := ctx.PollHTTP("https://poll.example.com", map[string]string{"X-Test": "1"}, 10)
	if err != nil {
		t.Fatalf("PollHTTP failed: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status = %d", resp.Status)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("expected sleep >= 10ms, got %v", elapsed)
	}

	if got := mock.CallSequence(); len(got) != 1 || got[0] != "HTTP" {
		t.Fatalf("unexpected call sequence: %v", got)
	}
}
