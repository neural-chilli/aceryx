package cases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func (s *CaseTypeService) RegisterCaseType(ctx context.Context, tenantID, createdBy uuid.UUID, name string, schema CaseTypeSchema) (CaseType, []ValidationError, error) {
	if name == "" {
		return CaseType{}, nil, fmt.Errorf("case type name is required")
	}
	if errs := validateSchemaDefinition(schema); len(errs) > 0 {
		return CaseType{}, errs, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CaseType{}, nil, fmt.Errorf("begin register case type tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextVersion int
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version), 0) + 1
FROM case_types
WHERE tenant_id = $1 AND name = $2
`, tenantID, name).Scan(&nextVersion); err != nil {
		return CaseType{}, nil, fmt.Errorf("resolve next case type version: %w", err)
	}

	rawSchema, err := json.Marshal(schema)
	if err != nil {
		return CaseType{}, nil, fmt.Errorf("marshal case type schema: %w", err)
	}

	var ct CaseType
	err = tx.QueryRowContext(ctx, `
INSERT INTO case_types (tenant_id, name, version, schema, status, created_by)
VALUES ($1, $2, $3, $4::jsonb, 'active', $5)
RETURNING id, tenant_id, name, version, schema, status, created_at, created_by
`, tenantID, name, nextVersion, string(rawSchema), createdBy).Scan(
		&ct.ID, &ct.TenantID, &ct.Name, &ct.Version, &rawSchema, &ct.Status, &ct.CreatedAt, &ct.CreatedBy,
	)
	if err != nil {
		return CaseType{}, nil, fmt.Errorf("insert case type: %w", err)
	}
	if err := json.Unmarshal(rawSchema, &ct.Schema); err != nil {
		return CaseType{}, nil, fmt.Errorf("unmarshal case type schema: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CaseType{}, nil, fmt.Errorf("commit register case type tx: %w", err)
	}
	return ct, nil, nil
}

func (s *CaseTypeService) ListCaseTypes(ctx context.Context, tenantID uuid.UUID, includeArchived bool) ([]CaseType, error) {
	query := `
SELECT id, tenant_id, name, version, schema, status, created_at, created_by
FROM case_types
WHERE tenant_id = $1
`
	if !includeArchived {
		query += ` AND status = 'active'`
	}
	query += ` ORDER BY name, version DESC`

	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list case types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseType, 0)
	for rows.Next() {
		var ct CaseType
		var raw []byte
		if err := rows.Scan(&ct.ID, &ct.TenantID, &ct.Name, &ct.Version, &raw, &ct.Status, &ct.CreatedAt, &ct.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan case type row: %w", err)
		}
		if err := json.Unmarshal(raw, &ct.Schema); err != nil {
			return nil, fmt.Errorf("decode case type schema: %w", err)
		}
		out = append(out, ct)
	}
	return out, rows.Err()
}

func (s *CaseTypeService) GetCaseTypeByID(ctx context.Context, tenantID, id uuid.UUID) (CaseType, error) {
	var ct CaseType
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, version, schema, status, created_at, created_by
FROM case_types
WHERE tenant_id = $1 AND id = $2
`, tenantID, id).Scan(&ct.ID, &ct.TenantID, &ct.Name, &ct.Version, &raw, &ct.Status, &ct.CreatedAt, &ct.CreatedBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CaseType{}, sql.ErrNoRows
		}
		return CaseType{}, fmt.Errorf("get case type: %w", err)
	}
	if err := json.Unmarshal(raw, &ct.Schema); err != nil {
		return CaseType{}, fmt.Errorf("decode case type schema: %w", err)
	}
	return ct, nil
}

func validateSchemaDefinition(schema CaseTypeSchema) []ValidationError {
	errs := make([]ValidationError, 0)
	for field, def := range schema.Fields {
		errs = append(errs, validateSchemaField(field, def)...)
	}
	return errs
}

func validateSchemaField(path string, def SchemaField) []ValidationError {
	validTypes := map[string]bool{"string": true, "number": true, "integer": true, "boolean": true, "object": true, "array": true, "text": true}
	errs := make([]ValidationError, 0)
	if def.Type == "" || !validTypes[def.Type] {
		errs = append(errs, ValidationError{Field: path, Rule: "type", Message: "unsupported field type in schema"})
	}
	for k, child := range def.Properties {
		errs = append(errs, validateSchemaField(path+"."+k, child)...)
	}
	return errs
}
