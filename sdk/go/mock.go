package sdk

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"
)

type MockContext struct {
	httpResponses      map[string]Response
	connectorResponses map[string]map[string]any
	secrets            map[string]string
	caseData           map[string]string
	configValues       map[string]string
	calls              []MockCall
}

type MockCall struct {
	Function  string
	Arguments []interface{}
	Timestamp time.Time
}

func NewMockContext() *MockContext {
	return &MockContext{
		httpResponses:      map[string]Response{},
		connectorResponses: map[string]map[string]any{},
		secrets:            map[string]string{},
		caseData:           map[string]string{},
		configValues:       map[string]string{},
		calls:              make([]MockCall, 0),
	}
}

func (m *MockContext) SetHTTPResponse(url string, resp Response) {
	m.httpResponses[url] = resp
}

func (m *MockContext) SetConnectorResponse(connectorID, operation string, result map[string]any) {
	m.connectorResponses[connectorID+":"+operation] = result
}

func (m *MockContext) SetSecret(key, value string) {
	m.secrets[key] = value
}

func (m *MockContext) SetCaseData(path, jsonValue string) {
	m.caseData[path] = jsonValue
}

func (m *MockContext) SetConfig(key, value string) {
	m.configValues[key] = value
}

func (m *MockContext) CallCount(function string) int {
	count := 0
	for _, call := range m.calls {
		if call.Function == function {
			count++
		}
	}
	return count
}

func (m *MockContext) CalledWith(function string, index int) []interface{} {
	count := 0
	for _, call := range m.calls {
		if call.Function != function {
			continue
		}
		if count == index {
			return call.Arguments
		}
		count++
	}
	return nil
}

func (m *MockContext) CallSequence() []string {
	sequence := make([]string, 0, len(m.calls))
	for _, call := range m.calls {
		sequence = append(sequence, call.Function)
	}
	return sequence
}

func (m *MockContext) AssertCalled(t *testing.T, function string) {
	t.Helper()
	if m.CallCount(function) == 0 {
		t.Fatalf("expected %s to be called", function)
	}
}

func (m *MockContext) AssertNotCalled(t *testing.T, function string) {
	t.Helper()
	if m.CallCount(function) > 0 {
		t.Fatalf("expected %s not to be called", function)
	}
}

func (m *MockContext) AssertCallOrder(t *testing.T, functions ...string) {
	t.Helper()
	if !slices.Equal(m.CallSequence(), functions) {
		t.Fatalf("unexpected call sequence: got %v want %v", m.CallSequence(), functions)
	}
}

func (m *MockContext) HTTP(req Request) (Response, error) {
	m.record("HTTP", req)
	resp, ok := m.httpResponses[req.URL]
	if !ok {
		return Response{}, fmt.Errorf("no mock HTTP response for %s", req.URL)
	}
	return resp, nil
}

func (m *MockContext) CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error) {
	m.record("CallConnector", connectorID, operation, input)
	out, ok := m.connectorResponses[connectorID+":"+operation]
	if !ok {
		return nil, fmt.Errorf("no mock connector response for %s:%s", connectorID, operation)
	}
	return out, nil
}

func (m *MockContext) CaseGet(path string) (string, error) {
	m.record("CaseGet", path)
	value, ok := m.caseData[path]
	if !ok {
		return "", fmt.Errorf("no mock case data for %s", path)
	}
	return value, nil
}

func (m *MockContext) CaseSet(path string, value interface{}) error {
	m.record("CaseSet", path, value)
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal case set value: %w", err)
	}
	m.caseData[path] = string(raw)
	return nil
}

func (m *MockContext) VaultRead(documentID string) ([]byte, error) {
	m.record("VaultRead", documentID)
	return nil, fmt.Errorf("not yet implemented")
}

func (m *MockContext) VaultWrite(filename, contentType string, data []byte) (string, error) {
	m.record("VaultWrite", filename, contentType, len(data))
	return "", fmt.Errorf("not yet implemented")
}

func (m *MockContext) Secret(key string) (string, error) {
	m.record("Secret", key)
	value, ok := m.secrets[key]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", key)
	}
	return value, nil
}

func (m *MockContext) Log(level LogLevel, msg string, args ...interface{}) {
	allArgs := make([]interface{}, 0, len(args)+2)
	allArgs = append(allArgs, level, msg)
	allArgs = append(allArgs, args...)
	m.record("Log", allArgs...)
}

func (m *MockContext) Config(key string) string {
	m.record("Config", key)
	return m.configValues[key]
}

func (m *MockContext) CreateCase(caseType string, data interface{}) (string, error) {
	m.record("CreateCase", caseType, data)
	return "case-mock-1", nil
}

func (m *MockContext) EmitEvent(eventType string, payload interface{}) error {
	m.record("EmitEvent", eventType, payload)
	return nil
}

func (m *MockContext) QueueConsume(driverID, topic string) (message []byte, metadata map[string]string, messageID string, err error) {
	m.record("QueueConsume", driverID, topic)
	return nil, nil, "", fmt.Errorf("not yet implemented")
}

func (m *MockContext) QueueAck(driverID, messageID string) error {
	m.record("QueueAck", driverID, messageID)
	return fmt.Errorf("not yet implemented")
}

func (m *MockContext) FileWatch(driverID, path string) (event FileEvent, err error) {
	m.record("FileWatch", driverID, path)
	return FileEvent{}, fmt.Errorf("not yet implemented")
}

func (m *MockContext) PollHTTP(url string, headers map[string]string, intervalMS int) (Response, error) {
	m.record("PollHTTP", url, headers, intervalMS)
	resp, err := m.HTTP(Request{
		Method:  "GET",
		URL:     url,
		Headers: headers,
	})
	if err != nil {
		return Response{}, err
	}
	if intervalMS > 0 {
		time.Sleep(time.Duration(intervalMS) * time.Millisecond)
	}
	return resp, nil
}

func (m *MockContext) CaseGetFloat(path string) float64 {
	raw, err := m.CaseGet(path)
	if err != nil {
		return 0
	}
	var n float64
	if err := json.Unmarshal([]byte(raw), &n); err == nil {
		return n
	}
	n, _ = strconv.ParseFloat(raw, 64)
	return n
}

func (m *MockContext) CaseGetString(path string) string {
	raw, err := m.CaseGet(path)
	if err != nil {
		return ""
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		return s
	}
	return raw
}

func (m *MockContext) CaseGetBool(path string) bool {
	raw, err := m.CaseGet(path)
	if err != nil {
		return false
	}
	var b bool
	if err := json.Unmarshal([]byte(raw), &b); err == nil {
		return b
	}
	b, _ = strconv.ParseBool(raw)
	return b
}

func (m *MockContext) record(function string, arguments ...interface{}) {
	m.calls = append(m.calls, MockCall{
		Function:  function,
		Arguments: arguments,
		Timestamp: time.Now(),
	})
}

var _ Context = (*MockContext)(nil)
var _ TriggerContext = (*MockContext)(nil)
