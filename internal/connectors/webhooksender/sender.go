package webhooksender

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/neural-chilli/aceryx/internal/connectors"
)

type Connector struct{}

func New() *Connector { return &Connector{} }

func (c *Connector) Meta() connectors.ConnectorMeta {
	return connectors.ConnectorMeta{Key: "webhook_sender", Name: "Webhook Sender", Description: "Outbound webhook delivery", Version: "v1", Icon: "pi pi-send"}
}

func (c *Connector) Auth() connectors.AuthSpec { return connectors.AuthSpec{Type: "none"} }

func (c *Connector) Triggers() []connectors.TriggerSpec { return nil }

func (c *Connector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{
		{
			Key:          "send",
			Name:         "Send",
			Description:  "Send JSON payload to webhook URL",
			InputSchema:  map[string]any{"type": "object"},
			OutputSchema: map[string]any{"type": "object"},
			Execute:      c.send,
		},
	}
}

func (c *Connector) send(ctx context.Context, _ map[string]string, input map[string]any) (map[string]any, error) {
	url, _ := input["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	headers := map[string]string{}
	if raw, ok := input["headers"].(map[string]any); ok {
		for k, v := range raw {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}
	timeout := 30 * time.Second
	if raw, ok := input["timeout_seconds"].(float64); ok && int(raw) > 0 {
		timeout = time.Duration(int(raw)) * time.Second
	}
	status, _, body, err := connectors.DoJSONRequest(ctx, http.MethodPost, url, headers, input["body"], timeout)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("webhook sender failed with status %d: %s", status, string(body))
	}
	return map[string]any{"status": status, "body": string(body)}, nil
}
