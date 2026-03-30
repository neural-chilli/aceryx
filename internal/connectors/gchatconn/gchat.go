package gchatconn

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
	return connectors.ConnectorMeta{Key: "google_chat", Name: "Google Chat", Description: "Google Chat webhook connector", Version: "v1", Icon: "pi pi-google"}
}

func (c *Connector) Auth() connectors.AuthSpec {
	return connectors.AuthSpec{Type: "api_key", Fields: []connectors.AuthField{{Key: "webhook_url", Label: "Webhook URL", Type: "url", Required: true}}}
}

func (c *Connector) Triggers() []connectors.TriggerSpec { return nil }

func (c *Connector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{
		{Key: "send_message", Name: "Send Message", Description: "Send message to Google Chat space", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"}, Execute: c.sendMessage},
	}
}

func (c *Connector) sendMessage(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	url := auth["webhook_url"]
	if url == "" {
		url, _ = input["webhook_url"].(string)
	}
	if url == "" {
		return nil, fmt.Errorf("webhook_url is required")
	}
	payload := map[string]any{"text": asString(input["text"])}
	status, _, body, err := connectors.DoJSONRequest(ctx, http.MethodPost, url, map[string]string{"Content-Type": "application/json"}, payload, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("google chat webhook status %d: %s", status, string(body))
	}
	return map[string]any{"status": status, "body": string(body)}, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
