package slackconn

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
	return connectors.ConnectorMeta{Key: "slack", Name: "Slack", Description: "Slack Web API connector", Version: "v1", Icon: "pi pi-comments"}
}

func (c *Connector) Auth() connectors.AuthSpec {
	return connectors.AuthSpec{Type: "oauth2", Fields: []connectors.AuthField{{Key: "bot_token", Label: "Bot Token", Type: "password", Required: true}, {Key: "api_base_url", Label: "API Base URL", Type: "url", Required: false}}}
}

func (c *Connector) Triggers() []connectors.TriggerSpec { return nil }

func (c *Connector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{
		{Key: "send_message", Name: "Send Message", Description: "Post message to a channel", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"}, Execute: c.sendMessage},
		{Key: "send_dm", Name: "Send DM", Description: "Send direct message to user", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"}, Execute: c.sendDM},
	}
}

func (c *Connector) sendMessage(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	base := auth["api_base_url"]
	if base == "" {
		base = "https://slack.com/api"
	}
	channel, _ := input["channel"].(string)
	text, _ := input["text"].(string)
	return c.callChatPostMessage(ctx, base, auth["bot_token"], map[string]any{"channel": channel, "text": text})
}

func (c *Connector) sendDM(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	base := auth["api_base_url"]
	if base == "" {
		base = "https://slack.com/api"
	}
	userID := asString(input["user"])
	if userID == "" {
		userID = asString(input["email"])
	}
	text := asString(input["text"])
	return c.callChatPostMessage(ctx, base, auth["bot_token"], map[string]any{"channel": userID, "text": text})
}

func (c *Connector) callChatPostMessage(ctx context.Context, base string, token string, payload map[string]any) (map[string]any, error) {
	headers := map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"}
	status, _, body, err := connectors.DoJSONRequest(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/chat.postMessage", headers, payload, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("slack api status %d: %s", status, string(body))
	}
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	if ok, _ := out["ok"].(bool); !ok {
		return nil, fmt.Errorf("slack api error: %v", out["error"])
	}
	return out, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
