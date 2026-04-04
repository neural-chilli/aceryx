package sdk

import (
	"encoding/json"
	"fmt"

	"github.com/neural-chilli/aceryx-plugin-sdk-go/sdk/internal/wasm"
)

type hostBridge interface {
	HTTP(req Request) (Response, error)
	CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error)
	CaseGet(path string) (string, error)
	CaseSet(path string, value interface{}) error
	VaultRead(documentID string) ([]byte, error)
	VaultWrite(filename, contentType string, data []byte) (string, error)
	Secret(key string) (string, error)
	Log(level LogLevel, message string, args ...interface{})
	Config(key string) string
	CreateCase(caseType string, data interface{}) (string, error)
	EmitEvent(eventType string, payload interface{}) error
	QueueConsume(driverID, topic string) ([]byte, map[string]string, string, error)
	QueueAck(driverID, messageID string) error
	FileWatch(driverID, path string) (FileEvent, error)
}

type contextImpl struct {
	host hostBridge
}

func NewContext() *contextImpl {
	return &contextImpl{host: newWASMHostBridge()}
}

func newContextWithBridge(host hostBridge) *contextImpl {
	return &contextImpl{host: host}
}

func (c *contextImpl) HTTP(req Request) (Response, error) {
	return c.host.HTTP(req)
}

func (c *contextImpl) CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error) {
	return c.host.CallConnector(connectorID, operation, input)
}

func (c *contextImpl) CaseGet(path string) (string, error) {
	return c.host.CaseGet(path)
}

func (c *contextImpl) CaseSet(path string, value interface{}) error {
	return c.host.CaseSet(path, value)
}

func (c *contextImpl) VaultRead(documentID string) ([]byte, error) {
	return c.host.VaultRead(documentID)
}

func (c *contextImpl) VaultWrite(filename, contentType string, data []byte) (string, error) {
	return c.host.VaultWrite(filename, contentType, data)
}

func (c *contextImpl) Secret(key string) (string, error) {
	return c.host.Secret(key)
}

func (c *contextImpl) Log(level LogLevel, msg string, args ...interface{}) {
	c.host.Log(level, msg, args...)
}

func (c *contextImpl) Config(key string) string {
	return c.host.Config(key)
}

type wasmHostBridge struct{}

func newWASMHostBridge() *wasmHostBridge {
	return &wasmHostBridge{}
}

func (w *wasmHostBridge) HTTP(req Request) (Response, error) {
	var out Response
	err := wasm.CallHTTP(req, &out)
	return out, err
}

func (w *wasmHostBridge) CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error) {
	payload := struct {
		ConnectorID string         `json:"connector_id"`
		Operation   string         `json:"operation"`
		Input       map[string]any `json:"input"`
	}{
		ConnectorID: connectorID,
		Operation:   operation,
		Input:       input,
	}
	out := map[string]any{}
	if err := wasm.CallConnector(payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (w *wasmHostBridge) CaseGet(path string) (string, error) {
	payload := struct {
		Path string `json:"path"`
	}{Path: path}
	var out struct {
		Value json.RawMessage `json:"value"`
	}
	if err := wasm.CallCaseGet(payload, &out); err != nil {
		return "", err
	}
	if len(out.Value) == 0 {
		return "", nil
	}
	return string(out.Value), nil
}

func (w *wasmHostBridge) CaseSet(path string, value interface{}) error {
	payload := struct {
		Path  string      `json:"path"`
		Value interface{} `json:"value"`
	}{Path: path, Value: value}
	return wasm.CallCaseSet(payload)
}

func (w *wasmHostBridge) VaultRead(documentID string) ([]byte, error) {
	payload := struct {
		DocumentID string `json:"document_id"`
	}{DocumentID: documentID}
	var out struct {
		Data []byte `json:"data"`
	}
	if err := wasm.CallVaultRead(payload, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (w *wasmHostBridge) VaultWrite(filename, contentType string, data []byte) (string, error) {
	payload := struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Data        []byte `json:"data"`
	}{Filename: filename, ContentType: contentType, Data: data}
	var out struct {
		DocumentID string `json:"document_id"`
	}
	if err := wasm.CallVaultWrite(payload, &out); err != nil {
		return "", err
	}
	return out.DocumentID, nil
}

func (w *wasmHostBridge) Secret(key string) (string, error) {
	payload := struct {
		Key string `json:"key"`
	}{Key: key}
	var out struct {
		Value string `json:"value"`
	}
	if err := wasm.CallSecretGet(payload, &out); err != nil {
		return "", err
	}
	if out.Value == "" {
		return "", fmt.Errorf("secret not found: %s", key)
	}
	return out.Value, nil
}

func (w *wasmHostBridge) Log(level LogLevel, message string, args ...interface{}) {
	payload := struct {
		Level   LogLevel `json:"level"`
		Message string   `json:"message"`
	}{Level: level, Message: fmt.Sprintf(message, args...)}
	_ = wasm.CallLog(payload)
}

func (w *wasmHostBridge) Config(key string) string {
	payload := struct {
		Key string `json:"key"`
	}{Key: key}
	var out struct {
		Value string `json:"value"`
	}
	if err := wasm.CallConfigGet(payload, &out); err != nil {
		return ""
	}
	return out.Value
}

func (w *wasmHostBridge) CreateCase(caseType string, data interface{}) (string, error) {
	payload := struct {
		CaseType string      `json:"case_type"`
		Data     interface{} `json:"data"`
	}{CaseType: caseType, Data: data}
	var out struct {
		CaseID string `json:"case_id"`
	}
	if err := wasm.CallCreateCase(payload, &out); err != nil {
		return "", err
	}
	return out.CaseID, nil
}

func (w *wasmHostBridge) EmitEvent(eventType string, payloadData interface{}) error {
	payload := struct {
		EventType string      `json:"event_type"`
		Payload   interface{} `json:"payload"`
	}{EventType: eventType, Payload: payloadData}
	return wasm.CallEmitEvent(payload)
}

func (w *wasmHostBridge) QueueConsume(driverID, topic string) ([]byte, map[string]string, string, error) {
	payload := struct {
		DriverID string `json:"driver_id"`
		Topic    string `json:"topic"`
	}{DriverID: driverID, Topic: topic}
	var out struct {
		Message   []byte            `json:"message"`
		Metadata  map[string]string `json:"metadata"`
		MessageID string            `json:"message_id"`
	}
	if err := wasm.CallQueueConsume(payload, &out); err != nil {
		return nil, nil, "", err
	}
	return out.Message, out.Metadata, out.MessageID, nil
}

func (w *wasmHostBridge) QueueAck(driverID, messageID string) error {
	payload := struct {
		DriverID  string `json:"driver_id"`
		MessageID string `json:"message_id"`
	}{DriverID: driverID, MessageID: messageID}
	return wasm.CallQueueAck(payload)
}

func (w *wasmHostBridge) FileWatch(driverID, path string) (FileEvent, error) {
	payload := struct {
		DriverID string `json:"driver_id"`
		Path     string `json:"path"`
	}{DriverID: driverID, Path: path}
	var out FileEvent
	err := wasm.CallFileWatch(payload, &out)
	return out, err
}
