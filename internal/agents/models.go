package agents

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/tasks"
)

const (
	defaultContextTimeout       = 30 * time.Second
	defaultSourceTimeout        = 10 * time.Second
	defaultContextMaxBytes      = 500 * 1024
	defaultLLMTimeout           = 120 * time.Second
	defaultKnowledgeTopK        = 5
	defaultValidationMaxAttempt = 3
)

type TaskCreator interface {
	CreateTaskFromActivation(ctx context.Context, caseID uuid.UUID, stepID string, cfg tasks.AssignmentConfig) error
}

type ExecutorConfig struct {
	DB              *sql.DB
	TaskCreator     TaskCreator
	LLMClient       *LLMClient
	Model           string
	ContextTimeout  time.Duration
	SourceTimeout   time.Duration
	ContextMaxBytes int
	LLMTimeout      time.Duration
}

type StepConfig struct {
	PromptTemplate      string                 `json:"prompt_template"`
	PromptVersion       int                    `json:"prompt_version"`
	Model               string                 `json:"model"`
	Context             []ContextSource        `json:"context"`
	OutputSchema        map[string]FieldDef    `json:"output_schema"`
	ConfidenceThreshold float64                `json:"confidence_threshold"`
	OnLowConfidence     string                 `json:"on_low_confidence"`
	MaxAttempts         int                    `json:"max_attempts"`
	AssignToRole        string                 `json:"assign_to_role"`
	AssignToUser        string                 `json:"assign_to_user"`
	SLAHours            int                    `json:"sla_hours"`
	Escalation          tasks.EscalationConfig `json:"escalation"`
	WritesCaseData      bool                   `json:"writes_case_data"`
	CaseDataField       string                 `json:"case_data_field"`
}

type ContextSource struct {
	Source        string   `json:"source"`
	Fields        []string `json:"fields"`
	Collection    string   `json:"collection"`
	Query         string   `json:"query"`
	TopK          int      `json:"top_k"`
	DocumentTypes []string `json:"document_types"`
}

type FieldDef struct {
	Type  string   `json:"type"`
	Enum  []any    `json:"enum"`
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Items string   `json:"items"`
}

type AssembledContext struct {
	Case      map[string]any      `json:"case,omitempty"`
	Steps     map[string]any      `json:"steps,omitempty"`
	Knowledge []KnowledgeResult   `json:"knowledge,omitempty"`
	Vault     []VaultDocResult    `json:"vault,omitempty"`
	Meta      ContextSnapshotMeta `json:"meta"`
}

type ContextSnapshotMeta struct {
	Sources        []SourceSnapshot `json:"sources"`
	TotalSizeBytes int              `json:"total_size_bytes"`
}

type SourceSnapshot struct {
	Source string   `json:"source"`
	Bytes  int      `json:"bytes"`
	Refs   []string `json:"refs,omitempty"`
}

type KnowledgeResult struct {
	DocumentID uuid.UUID `json:"document_id"`
	Filename   string    `json:"filename"`
	Text       string    `json:"text"`
	Similarity float64   `json:"similarity"`
}

type VaultDocResult struct {
	DocumentID    uuid.UUID       `json:"document_id"`
	Filename      string          `json:"filename"`
	MimeType      string          `json:"mime_type"`
	ExtractedText string          `json:"extracted_text"`
	Metadata      json.RawMessage `json:"metadata"`
}

type PromptTemplate struct {
	ID           uuid.UUID       `json:"id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	Name         string          `json:"name"`
	Version      int             `json:"version"`
	Template     string          `json:"template"`
	OutputSchema json.RawMessage `json:"output_schema"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
	CreatedBy    uuid.UUID       `json:"created_by"`
}

type CreatePromptTemplateRequest struct {
	Name         string         `json:"name"`
	Template     string         `json:"template"`
	OutputSchema map[string]any `json:"output_schema"`
	Metadata     map[string]any `json:"metadata"`
}

type UpdatePromptTemplateRequest struct {
	Template     string         `json:"template"`
	OutputSchema map[string]any `json:"output_schema"`
	Metadata     map[string]any `json:"metadata"`
}
