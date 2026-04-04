package sdk

import "encoding/json"

// Context is the primary interface for step plugin authors.
type Context interface {
	HTTP(req Request) (Response, error)
	CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error)
	CaseGet(path string) (string, error)
	CaseSet(path string, value interface{}) error
	VaultRead(documentID string) ([]byte, error)
	VaultWrite(filename, contentType string, data []byte) (string, error)
	Secret(key string) (string, error)
	Log(level LogLevel, msg string, args ...interface{})
	Config(key string) string
}

// TriggerContext extends Context for trigger plugins.
type TriggerContext interface {
	Context
	CreateCase(caseType string, data interface{}) (string, error)
	EmitEvent(eventType string, payload interface{}) error
	QueueConsume(driverID, topic string) (message []byte, metadata map[string]string, messageID string, err error)
	QueueAck(driverID, messageID string) error
	FileWatch(driverID, path string) (event FileEvent, err error)
	PollHTTP(url string, headers map[string]string, intervalMS int) (Response, error)
}

var returnedResult Result

// Return stores a final result for SDK-driven plugin wrappers.
func Return(result Result) {
	returnedResult = result
}

// Returned returns the last result passed to Return.
func Returned() Result {
	return returnedResult
}

//export aceryx_abi_version
func aceryx_abi_version() uint32 {
	return 1
}

// Result is the standard step-plugin result shape.
type Result struct {
	Status string          `json:"status"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
	Code   string          `json:"code,omitempty"`
}
