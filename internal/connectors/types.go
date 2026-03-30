package connectors

import (
	"context"
)

// Connector defines the interface that all connectors implement.
type Connector interface {
	Meta() ConnectorMeta
	Auth() AuthSpec
	Triggers() []TriggerSpec
	Actions() []ActionSpec
}

// ActionFunc executes one connector action.
type ActionFunc func(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error)

type ConnectorMeta struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Icon        string `json:"icon"`
}

type AuthSpec struct {
	Type   string      `json:"type"`
	Fields []AuthField `json:"fields"`
}

type AuthField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type TriggerSpec struct {
	Key          string         `json:"key"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Type         string         `json:"type"`
	OutputSchema map[string]any `json:"output_schema"`
}

type ActionSpec struct {
	Key          string         `json:"key"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema"`
	Execute      ActionFunc     `json:"-"`
}

type ActionSummary struct {
	Key          string         `json:"key"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema"`
}

type ConnectorDescriptor struct {
	Meta     ConnectorMeta   `json:"meta"`
	Auth     AuthSpec        `json:"auth"`
	Triggers []TriggerSpec   `json:"triggers"`
	Actions  []ActionSummary `json:"actions"`
}
