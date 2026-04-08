package workflows

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Workflow struct {
	ID                uuid.UUID          `json:"id"`
	Name              string             `json:"name"`
	CaseTypeID        string             `json:"case_type_id"`
	PublishedVersions []PublishedVersion `json:"published_versions,omitempty"`
}

type PublishedVersion struct {
	Version     int        `json:"version"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

type CreateRequest struct {
	Name       string `json:"name"`
	CaseTypeID string `json:"case_type_id"`
}

type Draft struct {
	WorkflowID  uuid.UUID       `json:"workflow_id"`
	VersionID   uuid.UUID       `json:"version_id"`
	Version     int             `json:"version"`
	AST         json.RawMessage `json:"ast"`
	YAMLSource  string          `json:"yaml_source,omitempty"`
	PublishedAt *time.Time      `json:"published_at,omitempty"`
}
