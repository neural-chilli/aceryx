package jiraconn

import (
	"context"
	"encoding/base64"
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
	return connectors.ConnectorMeta{Key: "jira", Name: "Jira", Description: "Jira issue connector", Version: "v1", Icon: "pi pi-ticket"}
}

func (c *Connector) Auth() connectors.AuthSpec {
	return connectors.AuthSpec{
		Type: "api_key",
		Fields: []connectors.AuthField{
			{Key: "base_url", Label: "Base URL", Type: "url", Required: true},
			{Key: "email", Label: "Email", Type: "string", Required: true},
			{Key: "api_token", Label: "API Token", Type: "password", Required: true},
		},
	}
}

func (c *Connector) Triggers() []connectors.TriggerSpec { return nil }

func (c *Connector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{
		{Key: "create_issue", Name: "Create Issue", Description: "Create a Jira issue", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"}, Execute: c.createIssue},
		{Key: "update_issue", Name: "Update Issue", Description: "Update issue fields", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"}, Execute: c.updateIssue},
		{Key: "transition_issue", Name: "Transition Issue", Description: "Transition issue state", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"}, Execute: c.transitionIssue},
	}
}

func (c *Connector) createIssue(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"fields": map[string]any{
			"project": map[string]any{"key": asString(input["project"])},
			"issuetype": map[string]any{
				"name": asString(input["issue_type"]),
			},
			"summary":     asString(input["summary"]),
			"description": asString(input["description"]),
		},
	}
	return c.call(ctx, auth, http.MethodPost, "/rest/api/3/issue", payload)
}

func (c *Connector) updateIssue(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	issueKey := asString(input["issue_key"])
	if issueKey == "" {
		return nil, fmt.Errorf("issue_key is required")
	}
	payload := map[string]any{"fields": input["fields"]}
	return c.call(ctx, auth, http.MethodPut, "/rest/api/3/issue/"+issueKey, payload)
}

func (c *Connector) transitionIssue(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	issueKey := asString(input["issue_key"])
	transitionID := asString(input["transition_id"])
	if issueKey == "" || transitionID == "" {
		return nil, fmt.Errorf("issue_key and transition_id are required")
	}
	payload := map[string]any{"transition": map[string]any{"id": transitionID}}
	return c.call(ctx, auth, http.MethodPost, "/rest/api/3/issue/"+issueKey+"/transitions", payload)
}

func (c *Connector) call(ctx context.Context, auth map[string]string, method string, path string, payload any) (map[string]any, error) {
	baseURL := strings.TrimRight(auth["base_url"], "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	email := auth["email"]
	token := auth["api_token"]
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token))
	headers := map[string]string{"Authorization": authHeader, "Content-Type": "application/json"}
	status, _, body, err := connectors.DoJSONRequest(ctx, method, baseURL+path, headers, payload, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("jira api status %d: %s", status, string(body))
	}
	out := map[string]any{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode jira response: %w", err)
	}
	if len(out) == 0 {
		out["status"] = status
	}
	return out, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
