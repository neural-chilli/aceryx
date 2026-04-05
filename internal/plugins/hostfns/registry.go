package hostfns

import (
	"fmt"
	"time"

	"github.com/neural-chilli/aceryx/internal/plugins"
)

type Registry struct {
	HTTP        *HTTPHost
	Connector   *ConnectorCaller
	Case        *CaseDataHost
	Vault       *VaultHost
	Secrets     *SecretGetter
	Queue       *QueueBridge
	FileWatcher *FileWatchBridge
	Logger      LoggerHost
	Events      *EventsHost
	Auditor     *Auditor
}

func (r *Registry) HTTPRequest(method, url string, headers map[string]string, body []byte, timeoutMS int) (plugins.HTTPResponse, error) {
	start := time.Now()
	resp, err := r.HTTP.HTTPRequest(method, url, headers, body, timeoutMS)
	if r.Auditor != nil {
		args := map[string]any{
			"method":      method,
			"url":         url,
			"timeout_ms":  timeoutMS,
			"status_code": resp.StatusCode,
			"duration_ms": time.Since(start).Milliseconds(),
		}
		if err != nil {
			args["error"] = err.Error()
		}
		r.Auditor.Record("HTTPRequest", start, err, args)
	}
	return resp, err
}

func (r *Registry) CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error) {
	start := time.Now()
	out, err := r.Connector.CallConnector(connectorID, operation, input)
	if r.Auditor != nil {
		r.Auditor.Record("CallConnector", start, err, map[string]any{"connector_id": connectorID, "operation": operation})
	}
	return out, err
}

func (r *Registry) CaseGet(path string) ([]byte, error) {
	start := time.Now()
	out, err := r.Case.CaseGet(path)
	if r.Auditor != nil {
		r.Auditor.Record("CaseGet", start, err, map[string]any{"path": path})
	}
	return out, err
}

func (r *Registry) CaseSet(path string, value []byte) error {
	start := time.Now()
	err := r.Case.CaseSet(path, value)
	if r.Auditor != nil {
		r.Auditor.Record("CaseSet", start, err, map[string]any{"path": path})
	}
	return err
}

func (r *Registry) VaultRead(documentID string) ([]byte, error) {
	start := time.Now()
	out, err := r.Vault.VaultRead(documentID)
	if r.Auditor != nil {
		r.Auditor.Record("VaultRead", start, err, map[string]any{"document_id": documentID})
	}
	return out, err
}

func (r *Registry) VaultWrite(filename, contentType string, data []byte) (string, error) {
	start := time.Now()
	out, err := r.Vault.VaultWrite(filename, contentType, data)
	if r.Auditor != nil {
		r.Auditor.Record("VaultWrite", start, err, map[string]any{"filename": filename, "content_type": contentType})
	}
	return out, err
}

func (r *Registry) SecretGet(key string) (string, error) {
	start := time.Now()
	out, err := r.Secrets.SecretGet(key)
	if r.Auditor != nil {
		r.Auditor.Record("SecretGet", start, err, map[string]any{"key": key, "redacted": true})
	}
	return out, err
}

func (r *Registry) Log(level, message string) {
	r.Logger.Log(level, message)
}

func (r *Registry) CreateCase(caseType string, data []byte) (string, error) {
	start := time.Now()
	out, err := r.Events.CreateCase(caseType, data)
	if r.Auditor != nil {
		r.Auditor.Record("CreateCase", start, err, map[string]any{"case_type": caseType})
	}
	return out, err
}

func (r *Registry) EmitEvent(eventType string, payload []byte) error {
	start := time.Now()
	err := r.Events.EmitEvent(eventType, payload)
	if r.Auditor != nil {
		r.Auditor.Record("EmitEvent", start, err, map[string]any{"event_type": eventType})
	}
	return err
}

func (r *Registry) QueueConsume(driverID string, config []byte, topic string) ([]byte, map[string]string, string, error) {
	start := time.Now()
	if r.Queue == nil {
		err := fmt.Errorf("queue host not configured")
		if r.Auditor != nil {
			r.Auditor.Record("QueueConsume", start, err, map[string]any{"driver_id": driverID, "topic": topic})
		}
		return nil, nil, "", err
	}
	msg, meta, messageID, err := r.Queue.Consume(driverID, config, topic)
	if r.Auditor != nil {
		r.Auditor.Record("QueueConsume", start, err, map[string]any{"driver_id": driverID, "topic": topic})
	}
	return msg, meta, messageID, err
}

func (r *Registry) QueueAck(driverID string, messageID string) error {
	start := time.Now()
	if r.Queue == nil {
		err := fmt.Errorf("queue host not configured")
		if r.Auditor != nil {
			r.Auditor.Record("QueueAck", start, err, map[string]any{"driver_id": driverID})
		}
		return err
	}
	err := r.Queue.Ack(driverID, messageID)
	if r.Auditor != nil {
		r.Auditor.Record("QueueAck", start, err, map[string]any{"driver_id": driverID})
	}
	return err
}

func (r *Registry) FileWatch(driverID string, config []byte, path string) (plugins.FileEvent, error) {
	start := time.Now()
	if r.FileWatcher == nil {
		err := fmt.Errorf("file watch host not configured")
		if r.Auditor != nil {
			r.Auditor.Record("FileWatch", start, err, map[string]any{"driver_id": driverID, "path": path})
		}
		return plugins.FileEvent{}, err
	}
	out, err := r.FileWatcher.Watch(driverID, config, path)
	if r.Auditor != nil {
		r.Auditor.Record("FileWatch", start, err, map[string]any{"driver_id": driverID, "path": path})
	}
	return out, err
}

var _ plugins.HostFunctions = (*Registry)(nil)
