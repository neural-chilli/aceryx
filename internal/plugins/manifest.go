package plugins

import "regexp"

var pluginIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

var knownManifestFields = map[string]struct{}{
	"id":               {},
	"name":             {},
	"version":          {},
	"type":             {},
	"category":         {},
	"tier":             {},
	"maturity":         {},
	"min_host_version": {},
	"max_host_version": {},
	"tool_capable":     {},
	"tool_description": {},
	"tool_safety":      {},
	"ui":               {},
	"host_functions":   {},
	"operational":      {},
	"cost":             {},
	"audit":            {},
	"trigger_contract": {},
	"trigger_config":   {},
}

type PluginManifest struct {
	ID       string `yaml:"id" json:"id"`
	Name     string `yaml:"name" json:"name"`
	Version  string `yaml:"version" json:"version"`
	Type     string `yaml:"type" json:"type"`
	Category string `yaml:"category" json:"category"`

	Tier     string `yaml:"tier" json:"tier"`
	Maturity string `yaml:"maturity" json:"maturity"`

	MinHostVersion string `yaml:"min_host_version" json:"min_host_version"`
	MaxHostVersion string `yaml:"max_host_version" json:"max_host_version"`

	ToolCapable     bool   `yaml:"tool_capable" json:"tool_capable"`
	ToolDescription string `yaml:"tool_description" json:"tool_description"`
	ToolSafety      string `yaml:"tool_safety" json:"tool_safety"`

	UI ManifestUI `yaml:"ui" json:"ui"`

	HostFunctions []string `yaml:"host_functions" json:"host_functions"`

	Operational *OperationalMeta `yaml:"operational" json:"operational,omitempty"`
	Cost        *CostMeta        `yaml:"cost" json:"cost,omitempty"`
	Audit       *AuditConfig     `yaml:"audit" json:"audit,omitempty"`

	TriggerContract *TriggerContract `yaml:"trigger_contract" json:"trigger_contract,omitempty"`
	TriggerConfig   *TriggerCfg      `yaml:"trigger_config" json:"trigger_config,omitempty"`

	Metadata map[string]any `yaml:"-" json:"metadata,omitempty"`
}

type ManifestUI struct {
	IconSVG         string        `yaml:"icon_svg" json:"icon_svg"`
	Description     string        `yaml:"description" json:"description"`
	LongDescription string        `yaml:"long_description" json:"long_description"`
	Properties      []PropertyDef `yaml:"properties" json:"properties"`
}

type PropertyDef struct {
	Key        string   `yaml:"key" json:"key"`
	Label      string   `yaml:"label" json:"label"`
	Type       string   `yaml:"type" json:"type"`
	Required   bool     `yaml:"required" json:"required"`
	Default    any      `yaml:"default" json:"default,omitempty"`
	Options    []string `yaml:"options" json:"options,omitempty"`
	HelpText   string   `yaml:"help_text" json:"help_text"`
	Validation string   `yaml:"validation" json:"validation"`
}

type OperationalMeta struct {
	RetrySemantics       string        `yaml:"retry_semantics" json:"retry_semantics"`
	TransactionGuarantee string        `yaml:"transaction_guarantee" json:"transaction_guarantee"`
	Idempotent           bool          `yaml:"idempotent" json:"idempotent"`
	RateLimited          bool          `yaml:"rate_limited" json:"rate_limited"`
	RateLimitConfig      *RateLimitCfg `yaml:"rate_limit_config" json:"rate_limit_config,omitempty"`
}

type RateLimitCfg struct {
	RequestsPerSecond float64 `yaml:"requests_per_second" json:"requests_per_second"`
	Burst             int     `yaml:"burst" json:"burst"`
}

type CostMeta struct {
	Level       string `yaml:"level" json:"level"`
	BillingUnit string `yaml:"billing_unit" json:"billing_unit"`
	Notes       string `yaml:"notes" json:"notes"`
}

type AuditConfig struct {
	HostCalls ManifestAuditHostCalls `yaml:"host_calls" json:"host_calls"`
}

type ManifestAuditHostCalls struct {
	Mode       string `yaml:"mode" json:"mode"`
	MaxEntries int    `yaml:"max_entries" json:"max_entries"`
	SampleRate int    `yaml:"sample_rate" json:"sample_rate"`
}

type TriggerContract struct {
	Delivery    string         `yaml:"delivery" json:"delivery"`
	State       string         `yaml:"state" json:"state"`
	Concurrency string         `yaml:"concurrency" json:"concurrency"`
	Ordering    string         `yaml:"ordering" json:"ordering"`
	Checkpoint  *CheckpointCfg `yaml:"checkpoint" json:"checkpoint,omitempty"`
}

type CheckpointCfg struct {
	Strategy   string `yaml:"strategy" json:"strategy"`
	IntervalMS int    `yaml:"interval_ms" json:"interval_ms"`
}

type TriggerCfg struct {
	PollingIntervalMS    int  `yaml:"polling_interval_ms" json:"polling_interval_ms"`
	ConfigurableInterval bool `yaml:"configurable_interval" json:"configurable_interval"`
}
