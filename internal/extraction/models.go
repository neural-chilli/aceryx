package extraction

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Schema struct {
	ID          uuid.UUID       `json:"id"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Fields      json.RawMessage `json:"fields"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type UpsertSchemaRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Fields      json.RawMessage `json:"fields"`
}

type Job struct {
	ID            uuid.UUID       `json:"id"`
	TenantID      uuid.UUID       `json:"tenant_id"`
	CaseID        uuid.UUID       `json:"case_id"`
	StepID        string          `json:"step_id"`
	DocumentID    uuid.UUID       `json:"document_id"`
	SchemaID      uuid.UUID       `json:"schema_id"`
	ModelUsed     string          `json:"model_used"`
	Status        string          `json:"status"`
	Confidence    *float64        `json:"confidence,omitempty"`
	ExtractedData json.RawMessage `json:"extracted_data,omitempty"`
	RawResponse   json.RawMessage `json:"raw_response,omitempty"`
	ProcessingMS  *int            `json:"processing_ms,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Field struct {
	ID             uuid.UUID `json:"id"`
	JobID          uuid.UUID `json:"job_id"`
	FieldName      string    `json:"field_name"`
	ExtractedValue string    `json:"extracted_value,omitempty"`
	Confidence     float64   `json:"confidence"`
	SourceText     string    `json:"source_text,omitempty"`
	PageNumber     *int      `json:"page_number,omitempty"`
	BBoxX          *float64  `json:"bbox_x,omitempty"`
	BBoxY          *float64  `json:"bbox_y,omitempty"`
	BBoxWidth      *float64  `json:"bbox_width,omitempty"`
	BBoxHeight     *float64  `json:"bbox_height,omitempty"`
	Status         string    `json:"status"`
	CorrectedValue string    `json:"corrected_value,omitempty"`
}

type Correction struct {
	ID             uuid.UUID `json:"id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	JobID          uuid.UUID `json:"job_id"`
	FieldName      string    `json:"field_name"`
	OriginalValue  string    `json:"original_value,omitempty"`
	CorrectedValue string    `json:"corrected_value"`
	Confidence     *float64  `json:"confidence,omitempty"`
	ModelUsed      string    `json:"model_used,omitempty"`
	CorrectedBy    uuid.UUID `json:"corrected_by"`
	CreatedAt      time.Time `json:"created_at"`
}

type CorrectFieldRequest struct {
	CorrectedValue string `json:"corrected_value"`
}
