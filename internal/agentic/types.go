package agentic

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	defaultMaxIterations = 10
	defaultMaxToolCalls  = 20
	defaultMaxTokens     = 50000
	defaultTimeout       = 5 * time.Minute
)

// ReasoningLimits defines hard boundaries enforced by the runtime.
type ReasoningLimits struct {
	MaxIterations int           `json:"max_iterations" yaml:"max_iterations"`
	MaxToolCalls  int           `json:"max_tool_calls" yaml:"max_tool_calls"`
	MaxTokens     int           `json:"max_tokens" yaml:"max_tokens"`
	Timeout       time.Duration `json:"timeout_seconds" yaml:"timeout_seconds"`
}

func (l *ReasoningLimits) UnmarshalJSON(data []byte) error {
	type alias struct {
		MaxIterations int     `json:"max_iterations"`
		MaxToolCalls  int     `json:"max_tool_calls"`
		MaxTokens     int     `json:"max_tokens"`
		Timeout       float64 `json:"timeout_seconds"`
	}
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	l.MaxIterations = v.MaxIterations
	l.MaxToolCalls = v.MaxToolCalls
	l.MaxTokens = v.MaxTokens
	if v.Timeout > 0 {
		l.Timeout = time.Duration(v.Timeout * float64(time.Second))
	}
	l.ApplyDefaults()
	return nil
}

func (l *ReasoningLimits) ApplyDefaults() {
	if l.MaxIterations <= 0 {
		l.MaxIterations = defaultMaxIterations
	}
	if l.MaxToolCalls <= 0 {
		l.MaxToolCalls = defaultMaxToolCalls
	}
	if l.MaxTokens <= 0 {
		l.MaxTokens = defaultMaxTokens
	}
	if l.Timeout <= 0 {
		l.Timeout = defaultTimeout
	}
}

// ToolPolicy governs what the agent is allowed to do with its tools.
type ToolPolicy struct {
	Tools           []ToolRef `json:"tools" yaml:"tools"`
	ToolMode        ToolMode  `json:"tool_mode" yaml:"tool_mode"`
	RequireApproval []string  `json:"require_approval" yaml:"require_approval"`
}

type ToolRef struct {
	Ref string `json:"ref" yaml:"ref"`
}

type ToolMode string

const (
	ToolModeReadOnly        ToolMode = "read_only"
	ToolModeRestrictedWrite ToolMode = "restricted_write"
	ToolModeFull            ToolMode = "full"
)

func (m ToolMode) Normalize() ToolMode {
	switch ToolMode(strings.TrimSpace(string(m))) {
	case ToolModeReadOnly, ToolModeRestrictedWrite, ToolModeFull:
		return m
	default:
		return ToolModeReadOnly
	}
}

// AgenticStepConfig is the full step configuration.
type AgenticStepConfig struct {
	Goal         string           `json:"goal" yaml:"goal"`
	ToolPolicy   ToolPolicy       `json:"tool_policy" yaml:"tool_policy"`
	Limits       ReasoningLimits  `json:"constraints" yaml:"constraints"`
	OutputSchema json.RawMessage  `json:"output_schema" yaml:"output_schema"`
	Escalation   EscalationConfig `json:"escalation" yaml:"escalation"`
	OutputPath   string           `json:"output_path" yaml:"output_path"`

	// ToolNodes are serialized by the builder with the step for execution-time manifest assembly.
	ToolNodes []ToolNodeConfig `json:"tool_nodes" yaml:"tool_nodes"`
}

func (c *AgenticStepConfig) ApplyDefaults() {
	c.Limits.ApplyDefaults()
	c.ToolPolicy.ToolMode = c.ToolPolicy.ToolMode.Normalize()
	if c.Escalation.ConfidenceThreshold <= 0 {
		c.Escalation.ConfidenceThreshold = 0.8
	}
	if strings.TrimSpace(c.Escalation.EscalateTo) == "" {
		c.Escalation.EscalateTo = "case_worker"
	}
	if strings.TrimSpace(c.OutputPath) == "" {
		c.OutputPath = "assessment"
	}
}

func (c AgenticStepConfig) Validate() error {
	if strings.TrimSpace(c.Goal) == "" {
		return fmt.Errorf("goal is required")
	}
	if len(c.OutputSchema) == 0 || strings.TrimSpace(string(c.OutputSchema)) == "" || strings.TrimSpace(string(c.OutputSchema)) == "null" {
		return fmt.Errorf("output_schema is required")
	}
	if len(c.ToolPolicy.Tools) == 0 {
		return fmt.Errorf("tool_policy.tools is required")
	}
	return nil
}

type EscalationConfig struct {
	ConfidenceThreshold float64 `json:"confidence_threshold" yaml:"confidence_threshold"`
	EscalateTo          string  `json:"escalate_to" yaml:"escalate_to"`
	IncludeTrace        bool    `json:"include_trace" yaml:"include_trace"`
}

type ToolNodeConfig struct {
	ID          string          `json:"id" yaml:"id"`
	Connector   string          `json:"connector" yaml:"connector"`
	Source      string          `json:"source" yaml:"source"`
	Config      json.RawMessage `json:"config" yaml:"config"`
	Description string          `json:"description" yaml:"description"`

	MCPServerURL string `json:"mcp_server_url" yaml:"mcp_server_url"`
	MCPToolName  string `json:"mcp_tool_name" yaml:"mcp_tool_name"`
	MCPPrefix    string `json:"mcp_prefix" yaml:"mcp_prefix"`

	KnowledgeBase string `json:"knowledge_base" yaml:"knowledge_base"`
}
