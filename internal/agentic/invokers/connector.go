package invokers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/plugins"
)

type PluginRuntime interface {
	ExecuteStep(ctx context.Context, ref plugins.PluginRef, input plugins.StepInput) (plugins.StepResult, error)
}

type ConnectorInvoker struct {
	pluginRuntime   PluginRuntime
	connectorID     string
	connectorConfig json.RawMessage
	timeout         time.Duration
}

func NewConnectorInvoker(pluginRuntime PluginRuntime, connectorID string, connectorConfig json.RawMessage, timeout time.Duration) *ConnectorInvoker {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ConnectorInvoker{
		pluginRuntime:   pluginRuntime,
		connectorID:     connectorID,
		connectorConfig: connectorConfig,
		timeout:         timeout,
	}
}

func (ci *ConnectorInvoker) Invoke(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if ci == nil || ci.pluginRuntime == nil {
		return nil, fmt.Errorf("connector invoker not configured")
	}
	payload := map[string]any{}
	if len(ci.connectorConfig) > 0 {
		if err := json.Unmarshal(ci.connectorConfig, &payload); err != nil {
			return nil, fmt.Errorf("decode connector config: %w", err)
		}
	}
	if len(args) > 0 {
		payload["arguments"] = json.RawMessage(args)
	}
	payloadRaw, _ := json.Marshal(payload)
	result, err := ci.pluginRuntime.ExecuteStep(ctx, plugins.PluginRef{ID: strings.TrimSpace(ci.connectorID)}, plugins.StepInput{
		Data:    payloadRaw,
		Timeout: ci.timeout,
	})
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(result.Status, "error") && strings.TrimSpace(result.Error) != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}
	if len(result.Output) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return result.Output, nil
}
