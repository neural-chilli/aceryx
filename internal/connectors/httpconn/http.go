package httpconn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/connectors"
)

type Connector struct{}

func New() *Connector { return &Connector{} }

func (c *Connector) Meta() connectors.ConnectorMeta {
	return connectors.ConnectorMeta{Key: "http", Name: "HTTP/REST", Description: "Generic HTTP connector", Version: "v1", Icon: "pi pi-globe"}
}

func (c *Connector) Auth() connectors.AuthSpec { return connectors.AuthSpec{Type: "none"} }

func (c *Connector) Triggers() []connectors.TriggerSpec { return nil }

func (c *Connector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{
		{
			Key:          "request",
			Name:         "Request",
			Description:  "Make an HTTP request",
			InputSchema:  map[string]any{"type": "object"},
			OutputSchema: map[string]any{"type": "object"},
			Execute:      c.request,
		},
	}
}

func (c *Connector) request(ctx context.Context, _ map[string]string, input map[string]any) (map[string]any, error) {
	method := strings.ToUpper(readString(input, "method", "GET"))
	url := readString(input, "url", "")
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	headers := readStringMap(input["headers"])
	timeout := time.Duration(readInt(input, "timeout_seconds", 30)) * time.Second
	status, responseHeaders, body, err := connectors.DoJSONRequest(ctx, method, url, headers, input["body"], timeout)
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("http request failed with status %d: %s", status, string(body))
	}

	parsedBody := any(string(body))
	if len(body) > 0 {
		tmp := any(nil)
		if jerr := json.Unmarshal(body, &tmp); jerr == nil {
			parsedBody = tmp
		}
	}
	return map[string]any{
		"status":  status,
		"headers": flattenHeaders(responseHeaders),
		"body":    parsedBody,
	}, nil
}

func readString(input map[string]any, key string, fallback string) string {
	raw, ok := input[key]
	if !ok || raw == nil {
		return fallback
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return fallback
}

func readInt(input map[string]any, key string, fallback int) int {
	raw, ok := input[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return fallback
}

func readStringMap(raw any) map[string]string {
	out := map[string]string{}
	switch typed := raw.(type) {
	case map[string]string:
		return typed
	case map[string]any:
		for k, v := range typed {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func flattenHeaders(headers http.Header) map[string]any {
	out := map[string]any{}
	for key, values := range headers {
		if len(values) == 1 {
			out[key] = values[0]
		} else {
			items := make([]any, len(values))
			for i, v := range values {
				items[i] = v
			}
			out[key] = items
		}
	}
	return out
}
