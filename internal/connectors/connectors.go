// Package connectors defines the Connector interface and provides
// the connector registry and built-in connector implementations.
package connectors

// Connector defines the interface that all connectors must implement.
type Connector interface {
	Meta() ConnectorMeta
	Auth() AuthSpec
	Triggers() []TriggerSpec
	Actions() []ActionSpec
}

// ConnectorMeta describes a connector.
type ConnectorMeta struct {
	Key         string
	Name        string
	Description string
	Version     string
	Icon        string
}

// AuthSpec describes a connector's authentication requirements.
type AuthSpec struct {
	Type   string // "none", "api_key", "oauth2", "basic"
	Fields []AuthField
}

// AuthField describes a single auth configuration field.
type AuthField struct {
	Key      string
	Label    string
	Type     string // "string", "password", "url"
	Required bool
}

// TriggerSpec describes an inbound trigger a connector provides.
type TriggerSpec struct {
	Key         string
	Name        string
	Description string
	Type        string // "webhook", "polling", "scheduled"
}

// ActionSpec describes an action a connector can perform.
type ActionSpec struct {
	Key          string
	Name         string
	Description  string
	InputSchema  map[string]any
	OutputSchema map[string]any
}
