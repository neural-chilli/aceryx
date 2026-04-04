package sdk

import "testing"

func TestMockContextRecordsCallsAndAssertions(t *testing.T) {
	mock := NewMockContext()
	mock.SetHTTPResponse("https://example.com", Response{Status: 200, Body: []byte(`{"ok":true}`)})
	mock.SetSecret("api_key", "sk-123")
	mock.SetCaseData("path", `"value"`)
	mock.SetConfig("enabled", "true")
	mock.SetConnectorResponse("companies-house", "lookup", map[string]any{"name": "Acme"})

	if _, err := mock.HTTP(Request{Method: "GET", URL: "https://example.com"}); err != nil {
		t.Fatalf("HTTP failed: %v", err)
	}
	if _, err := mock.Secret("api_key"); err != nil {
		t.Fatalf("Secret failed: %v", err)
	}
	if _, err := mock.CaseGet("path"); err != nil {
		t.Fatalf("CaseGet failed: %v", err)
	}
	if _, err := mock.CallConnector("companies-house", "lookup", nil); err != nil {
		t.Fatalf("CallConnector failed: %v", err)
	}
	mock.Log(LogInfo, "hello %s", "world")

	mock.AssertCalled(t, "HTTP")
	mock.AssertCalled(t, "Secret")
	mock.AssertNotCalled(t, "VaultRead")
	mock.AssertCallOrder(t, "HTTP", "Secret", "CaseGet", "CallConnector", "Log")

	if got := mock.CallCount("HTTP"); got != 1 {
		t.Fatalf("HTTP count = %d", got)
	}
	args := mock.CalledWith("HTTP", 0)
	if len(args) != 1 {
		t.Fatalf("HTTP args len = %d", len(args))
	}
}
