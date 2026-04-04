package hostfns

import "fmt"

type EventEmitter interface {
	CreateCase(caseType string, data []byte) (string, error)
	EmitEvent(eventType string, payload []byte) error
}

type EventsHost struct {
	Emitter EventEmitter
}

func (h *EventsHost) CreateCase(caseType string, data []byte) (string, error) {
	if h.Emitter == nil {
		return "", fmt.Errorf("event host not configured")
	}
	return h.Emitter.CreateCase(caseType, data)
}

func (h *EventsHost) EmitEvent(eventType string, payload []byte) error {
	if h.Emitter == nil {
		return fmt.Errorf("event host not configured")
	}
	return h.Emitter.EmitEvent(eventType, payload)
}
