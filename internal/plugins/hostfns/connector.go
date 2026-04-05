package hostfns

import (
	"context"
	"fmt"
	"time"

	"github.com/neural-chilli/aceryx/internal/connectors"
	httpfw "github.com/neural-chilli/aceryx/internal/http"
)

type HTTPConnectorSpec struct {
	Request       httpfw.PluginHTTPRequest
	ParseResponse func(resp httpfw.PluginHTTPResponse) (map[string]any, error)
}

type HTTPConnectorResolver interface {
	ResolveHTTPConnector(tenantID, pluginID, connectorID, operation string, input map[string]any) (*HTTPConnectorSpec, error)
}

type ConnectorCaller struct {
	Registry      *connectors.Registry
	Timeout       time.Duration
	ClientManager *httpfw.ClientManager
	TenantID      string
	PluginID      string
	Resolver      HTTPConnectorResolver
}

func (c *ConnectorCaller) CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error) {
	if c.Resolver != nil && c.ClientManager != nil {
		spec, err := c.Resolver.ResolveHTTPConnector(c.TenantID, c.PluginID, connectorID, operation, input)
		if err != nil {
			return nil, err
		}
		if spec != nil {
			timeout := c.Timeout
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			spec.Request.TenantID = c.TenantID
			spec.Request.PluginID = c.PluginID
			resp, err := c.ClientManager.Execute(ctx, spec.Request)
			if err != nil {
				return nil, err
			}
			if spec.ParseResponse == nil {
				return map[string]any{"status": resp.Status, "body": string(resp.Body)}, nil
			}
			return spec.ParseResponse(resp)
		}
	}

	if c.Registry == nil {
		return nil, fmt.Errorf("connector not found: %s", connectorID)
	}
	action, ok := c.Registry.GetAction(connectorID, operation)
	if !ok {
		return nil, fmt.Errorf("connector not found: %s", connectorID)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return action.Execute(ctx, map[string]string{}, input)
}
