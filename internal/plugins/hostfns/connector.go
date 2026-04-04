package hostfns

import (
	"context"
	"fmt"
	"time"

	"github.com/neural-chilli/aceryx/internal/connectors"
)

type ConnectorCaller struct {
	Registry *connectors.Registry
	Timeout  time.Duration
}

func (c *ConnectorCaller) CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error) {
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
