package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tetratelabs/wazero"
)

type PluginRef struct {
	ID      string
	Version string
}

var ErrInvalidPluginRef = errors.New("invalid plugin ref")

func ParsePluginRef(s string) PluginRef {
	s = strings.TrimSpace(s)
	if s == "" {
		return PluginRef{}
	}
	parts := strings.Split(s, "@")
	if len(parts) == 1 {
		return PluginRef{ID: strings.TrimSpace(parts[0])}
	}
	if len(parts) == 2 {
		return PluginRef{ID: strings.TrimSpace(parts[0]), Version: strings.TrimSpace(parts[1])}
	}
	return PluginRef{}
}

func ParsePluginRefStrict(s string) (PluginRef, error) {
	ref := ParsePluginRef(s)
	if ref.ID == "" {
		return PluginRef{}, fmt.Errorf("%w: plugin id is required", ErrInvalidPluginRef)
	}
	if strings.Count(s, "@") > 1 {
		return PluginRef{}, fmt.Errorf("%w: too many @ separators", ErrInvalidPluginRef)
	}
	if strings.Contains(s, "@") && ref.Version == "" {
		return PluginRef{}, fmt.Errorf("%w: plugin version is required when @ is present", ErrInvalidPluginRef)
	}
	return ref, nil
}

type PluginType string

const (
	StepPlugin    PluginType = "step"
	TriggerPlugin PluginType = "trigger"
)

type PluginStatus string

const (
	PluginActive   PluginStatus = "active"
	PluginDisabled PluginStatus = "disabled"
	PluginError    PluginStatus = "error"
)

type PluginManifest struct {
	ID             string         `yaml:"id" json:"id"`
	Name           string         `yaml:"name" json:"name"`
	Version        string         `yaml:"version" json:"version"`
	Type           PluginType     `yaml:"type" json:"type"`
	Category       string         `yaml:"category" json:"category"`
	Tier           string         `yaml:"tier" json:"tier"`
	Maturity       string         `yaml:"maturity" json:"maturity"`
	MinHostVersion string         `yaml:"min_host_version" json:"min_host_version"`
	ToolCapable    bool           `yaml:"tool_capable" json:"tool_capable"`
	ToolSafety     string         `yaml:"tool_safety" json:"tool_safety"`
	Audit          ManifestAudit  `yaml:"audit" json:"audit"`
	Metadata       map[string]any `yaml:"-" json:"metadata,omitempty"`
}

type ManifestAudit struct {
	HostCalls ManifestAuditHostCalls `yaml:"host_calls" json:"host_calls"`
}

type ManifestAuditHostCalls struct {
	Mode       string `yaml:"mode" json:"mode"`
	MaxEntries int    `yaml:"max_entries" json:"max_entries"`
	SampleRate int    `yaml:"sample_rate" json:"sample_rate"`
}

type Plugin struct {
	ID           string
	Name         string
	Version      string
	Type         PluginType
	Category     string
	LicenceTier  string
	MaturityTier string
	ToolCapable  bool
	ToolSafety   string
	Manifest     PluginManifest
	Module       wazero.CompiledModule
	WASMHash     string
	ManifestHash string
	IsLatest     bool
	Status       PluginStatus
	ErrorMessage string
}

type StepInput struct {
	TenantID    uuid.UUID       `json:"tenant_id"`
	CaseID      uuid.UUID       `json:"case_id,omitempty"`
	StepID      string          `json:"step_id,omitempty"`
	Data        json.RawMessage `json:"data"`
	Timeout     time.Duration   `json:"-"`
	AuditMode   string          `json:"-"`
	AuditSample int             `json:"-"`
}

type StepResult struct {
	Status string          `json:"status"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type TriggerConfig struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Config   []byte    `json:"config"`
}

type FileEvent struct {
	Path      string            `json:"path"`
	Operation string            `json:"operation"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

type HostFunctions interface {
	HTTPRequest(method, url string, headers map[string]string, body []byte, timeoutMS int) (HTTPResponse, error)
	CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error)
	CaseGet(path string) ([]byte, error)
	CaseSet(path string, value []byte) error
	VaultRead(documentID string) ([]byte, error)
	VaultWrite(filename, contentType string, data []byte) (string, error)
	SecretGet(key string) (string, error)
	Log(level, message string)
	CreateCase(caseType string, data []byte) (string, error)
	EmitEvent(eventType string, payload []byte) error
	QueueConsume(driverID string, config []byte, topic string) ([]byte, map[string]string, string, error)
	QueueAck(driverID string, messageID string) error
	FileWatch(driverID string, config []byte, path string) (FileEvent, error)
}

type LicenceKey interface {
	AllowsCommercialPlugin(pluginID string) bool
}

type AllowAllLicence struct{}

func (AllowAllLicence) AllowsCommercialPlugin(string) bool { return true }

type PluginRuntime interface {
	LoadAll(pluginsDir string, licence LicenceKey) error
	Load(pluginDir string, licence LicenceKey) (*Plugin, error)
	Unload(ref PluginRef) error
	Reload(ref PluginRef) error
	ExecuteStep(ctx context.Context, ref PluginRef, input StepInput) (StepResult, error)
	StartTrigger(ref PluginRef, config TriggerConfig) error
	StopTrigger(ref PluginRef) error
	List() []*Plugin
	Get(ref PluginRef) (*Plugin, error)
	ListVersions(pluginID string) ([]*Plugin, error)
}
