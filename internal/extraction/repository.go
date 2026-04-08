package extraction

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListSchemas(ctx context.Context, tenantID uuid.UUID) ([]Schema, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, tenant_id, name, description, fields, created_at, updated_at
FROM extraction_schemas
WHERE tenant_id = $1
ORDER BY name ASC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list extraction schemas: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Schema, 0)
	for rows.Next() {
		item, err := scanSchema(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extraction schemas: %w", err)
	}
	return out, nil
}

func (r *Repository) GetSchema(ctx context.Context, tenantID, id uuid.UUID) (Schema, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, description, fields, created_at, updated_at
FROM extraction_schemas
WHERE tenant_id = $1 AND id = $2
`, tenantID, id)
	return scanSchema(row)
}

func (r *Repository) CreateSchema(ctx context.Context, tenantID uuid.UUID, req UpsertSchemaRequest) (Schema, error) {
	row := r.db.QueryRowContext(ctx, `
INSERT INTO extraction_schemas (tenant_id, name, description, fields)
VALUES ($1, $2, $3, $4::jsonb)
RETURNING id, tenant_id, name, description, fields, created_at, updated_at
`, tenantID, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), []byte(req.Fields))
	return scanSchema(row)
}

func (r *Repository) UpdateSchema(ctx context.Context, tenantID, id uuid.UUID, req UpsertSchemaRequest) (Schema, error) {
	row := r.db.QueryRowContext(ctx, `
UPDATE extraction_schemas
SET name = $3,
    description = $4,
    fields = $5::jsonb,
    updated_at = now()
WHERE tenant_id = $1
  AND id = $2
RETURNING id, tenant_id, name, description, fields, created_at, updated_at
`, tenantID, id, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), []byte(req.Fields))
	return scanSchema(row)
}

func (r *Repository) DeleteSchema(ctx context.Context, tenantID, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
DELETE FROM extraction_schemas
WHERE tenant_id = $1
  AND id = $2
`, tenantID, id)
	if err != nil {
		return fmt.Errorf("delete extraction schema: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) GetJob(ctx context.Context, tenantID, id uuid.UUID) (Job, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, tenant_id, case_id, step_id, document_id, schema_id, model_used, status,
       confidence::float8, extracted_data, raw_response, processing_ms, created_at, updated_at
FROM extraction_jobs
WHERE tenant_id = $1
  AND id = $2
`, tenantID, id)
	return scanJob(row)
}

func (r *Repository) ListFields(ctx context.Context, tenantID, jobID uuid.UUID) ([]Field, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT f.id, f.job_id, f.field_name, COALESCE(f.extracted_value, ''), f.confidence::float8,
       COALESCE(f.source_text, ''), f.page_number,
       f.bbox_x::float8, f.bbox_y::float8, f.bbox_width::float8, f.bbox_height::float8,
       f.status, COALESCE(f.corrected_value, '')
FROM extraction_fields f
JOIN extraction_jobs j ON j.id = f.job_id
WHERE j.tenant_id = $1
  AND j.id = $2
ORDER BY f.field_name ASC
`, tenantID, jobID)
	if err != nil {
		return nil, fmt.Errorf("list extraction fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Field, 0)
	for rows.Next() {
		item, err := scanField(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extraction fields: %w", err)
	}
	return out, nil
}

func (r *Repository) AcceptJob(ctx context.Context, tenantID, jobID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE extraction_jobs
SET status = 'accepted',
    updated_at = now()
WHERE tenant_id = $1
  AND id = $2
`, tenantID, jobID)
	if err != nil {
		return fmt.Errorf("accept extraction job: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) RejectJob(ctx context.Context, tenantID, jobID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE extraction_jobs
SET status = 'rejected',
    updated_at = now()
WHERE tenant_id = $1
  AND id = $2
`, tenantID, jobID)
	if err != nil {
		return fmt.Errorf("reject extraction job: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ConfirmField(ctx context.Context, tenantID, fieldID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE extraction_fields f
SET status = 'confirmed',
    corrected_value = NULL
FROM extraction_jobs j
WHERE f.id = $2
  AND f.job_id = j.id
  AND j.tenant_id = $1
`, tenantID, fieldID)
	if err != nil {
		return fmt.Errorf("confirm extraction field: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) RejectField(ctx context.Context, tenantID, fieldID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE extraction_fields f
SET status = 'rejected'
FROM extraction_jobs j
WHERE f.id = $2
  AND f.job_id = j.id
  AND j.tenant_id = $1
`, tenantID, fieldID)
	if err != nil {
		return fmt.Errorf("reject extraction field: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) CorrectField(ctx context.Context, tenantID, fieldID uuid.UUID, correctedValue string, correctedBy uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin correction transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var jobID uuid.UUID
	var fieldName string
	var originalValue sql.NullString
	var confidence sql.NullFloat64
	var modelUsed string
	err = tx.QueryRowContext(ctx, `
SELECT f.job_id,
       f.field_name,
       f.extracted_value,
       f.confidence::float8,
       j.model_used
FROM extraction_fields f
JOIN extraction_jobs j ON j.id = f.job_id
WHERE f.id = $2
  AND j.tenant_id = $1
FOR UPDATE
`, tenantID, fieldID).Scan(&jobID, &fieldName, &originalValue, &confidence, &modelUsed)
	if err != nil {
		if err == sql.ErrNoRows {
			return err
		}
		return fmt.Errorf("load field correction context: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE extraction_fields
SET status = 'corrected',
    corrected_value = $2
WHERE id = $1
`, fieldID, strings.TrimSpace(correctedValue)); err != nil {
		return fmt.Errorf("update corrected extraction field: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO extraction_corrections (
    tenant_id,
    job_id,
    field_name,
    original_value,
    corrected_value,
    confidence,
    model_used,
    corrected_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, tenantID, jobID, fieldName, nullStringValue(originalValue), strings.TrimSpace(correctedValue), nullFloatValue(confidence), modelUsed, correctedBy)
	if err != nil {
		return fmt.Errorf("insert extraction correction: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit correction transaction: %w", err)
	}
	return nil
}

func (r *Repository) ListCorrections(ctx context.Context, tenantID uuid.UUID, schemaID *uuid.UUID, since *time.Time) ([]Correction, error) {
	query := `
SELECT c.id, c.tenant_id, c.job_id, c.field_name,
       COALESCE(c.original_value, ''), c.corrected_value,
       c.confidence::float8, COALESCE(c.model_used, ''), c.corrected_by, c.created_at
FROM extraction_corrections c
JOIN extraction_jobs j ON j.id = c.job_id
WHERE c.tenant_id = $1
`
	args := []any{tenantID}
	idx := 2
	if schemaID != nil {
		query += fmt.Sprintf(" AND j.schema_id = $%d", idx)
		args = append(args, *schemaID)
		idx++
	}
	if since != nil {
		query += fmt.Sprintf(" AND c.created_at >= $%d", idx)
		args = append(args, *since)
		idx++
	}
	query += " ORDER BY c.created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list extraction corrections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Correction, 0)
	for rows.Next() {
		item, err := scanCorrection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extraction corrections: %w", err)
	}
	return out, nil
}

func (r *Repository) GetReviewOutputPath(ctx context.Context, caseID uuid.UUID, stepID string) (string, error) {
	var out sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT metadata->'extraction_review'->>'output_path'
FROM case_steps
WHERE case_id = $1
  AND step_id = $2
`, caseID, stepID).Scan(&out)
	if err != nil {
		return "", fmt.Errorf("load extraction review output_path: %w", err)
	}
	if !out.Valid {
		return "", nil
	}
	return strings.TrimSpace(out.String), nil
}

type schemaScanner interface {
	Scan(dest ...any) error
}

type jobScanner interface {
	Scan(dest ...any) error
}

type fieldScanner interface {
	Scan(dest ...any) error
}

type correctionScanner interface {
	Scan(dest ...any) error
}

func scanSchema(row schemaScanner) (Schema, error) {
	var item Schema
	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.Name,
		&item.Description,
		&item.Fields,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Schema{}, err
	}
	return item, nil
}

func scanJob(row jobScanner) (Job, error) {
	var item Job
	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.CaseID,
		&item.StepID,
		&item.DocumentID,
		&item.SchemaID,
		&item.ModelUsed,
		&item.Status,
		&item.Confidence,
		&item.ExtractedData,
		&item.RawResponse,
		&item.ProcessingMS,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Job{}, err
	}
	return item, nil
}

func scanField(row fieldScanner) (Field, error) {
	var item Field
	if err := row.Scan(
		&item.ID,
		&item.JobID,
		&item.FieldName,
		&item.ExtractedValue,
		&item.Confidence,
		&item.SourceText,
		&item.PageNumber,
		&item.BBoxX,
		&item.BBoxY,
		&item.BBoxWidth,
		&item.BBoxHeight,
		&item.Status,
		&item.CorrectedValue,
	); err != nil {
		return Field{}, err
	}
	return item, nil
}

func scanCorrection(row correctionScanner) (Correction, error) {
	var item Correction
	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.JobID,
		&item.FieldName,
		&item.OriginalValue,
		&item.CorrectedValue,
		&item.Confidence,
		&item.ModelUsed,
		&item.CorrectedBy,
		&item.CreatedAt,
	); err != nil {
		return Correction{}, err
	}
	return item, nil
}

func nullStringValue(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func nullFloatValue(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}
