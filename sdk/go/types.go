package sdk

import (
	"encoding/json"
	"time"
)

type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

type Request struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
	Timeout int               `json:"timeout"`
}

type Response struct {
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
}

func (r Response) JSON() (map[string]interface{}, error) {
	if len(r.Body) == 0 {
		return map[string]interface{}{}, nil
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(r.Body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r Response) Text() string {
	return string(r.Body)
}

type FileEvent struct {
	Path      string    `json:"path"`
	EventType string    `json:"event_type"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
}
