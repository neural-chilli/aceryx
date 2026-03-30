package agents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type PromptTemplateService struct {
	db *sql.DB
}

func NewPromptTemplateService(db *sql.DB) *PromptTemplateService {
	return &PromptTemplateService{db: db}
}

func (s *PromptTemplateService) List(ctx context.Context, tenantID uuid.UUID) ([]PromptTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, name, version, template, COALESCE(output_schema, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb), created_at, created_by
FROM prompt_templates
WHERE tenant_id = $1
ORDER BY name, version DESC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list prompt templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PromptTemplate, 0)
	for rows.Next() {
		var item PromptTemplate
		if err := rows.Scan(&item.ID, &item.TenantID, &item.Name, &item.Version, &item.Template, &item.OutputSchema, &item.Metadata, &item.CreatedAt, &item.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan prompt template row: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt templates: %w", err)
	}
	return out, nil
}

func (s *PromptTemplateService) Create(ctx context.Context, tenantID, createdBy uuid.UUID, req CreatePromptTemplateRequest) (PromptTemplate, error) {
	if strings.TrimSpace(req.Name) == "" {
		return PromptTemplate{}, fmt.Errorf("template name is required")
	}
	if strings.TrimSpace(req.Template) == "" {
		return PromptTemplate{}, fmt.Errorf("template content is required")
	}
	outSchemaRaw, _ := json.Marshal(req.OutputSchema)
	metadataRaw, _ := json.Marshal(req.Metadata)
	var item PromptTemplate
	err := s.db.QueryRowContext(ctx, `
INSERT INTO prompt_templates (tenant_id, name, version, template, output_schema, metadata, created_by)
VALUES ($1, $2, 1, $3, $4::jsonb, $5::jsonb, $6)
RETURNING id, tenant_id, name, version, template, COALESCE(output_schema, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb), created_at, created_by
`, tenantID, req.Name, req.Template, string(outSchemaRaw), string(metadataRaw), createdBy).Scan(
		&item.ID,
		&item.TenantID,
		&item.Name,
		&item.Version,
		&item.Template,
		&item.OutputSchema,
		&item.Metadata,
		&item.CreatedAt,
		&item.CreatedBy,
	)
	if err != nil {
		return PromptTemplate{}, fmt.Errorf("create prompt template: %w", err)
	}
	return item, nil
}

func (s *PromptTemplateService) GetVersion(ctx context.Context, tenantID uuid.UUID, name string, version int) (PromptTemplate, error) {
	var item PromptTemplate
	err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, version, template, COALESCE(output_schema, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb), created_at, created_by
FROM prompt_templates
WHERE tenant_id = $1 AND name = $2 AND version = $3
`, tenantID, name, version).Scan(
		&item.ID,
		&item.TenantID,
		&item.Name,
		&item.Version,
		&item.Template,
		&item.OutputSchema,
		&item.Metadata,
		&item.CreatedAt,
		&item.CreatedBy,
	)
	if err != nil {
		return PromptTemplate{}, err
	}
	return item, nil
}

func (s *PromptTemplateService) CreateVersion(ctx context.Context, tenantID, createdBy uuid.UUID, name string, req UpdatePromptTemplateRequest) (PromptTemplate, error) {
	if strings.TrimSpace(req.Template) == "" {
		return PromptTemplate{}, fmt.Errorf("template content is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PromptTemplate{}, fmt.Errorf("begin create prompt template version tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextVersion int
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version), 0) + 1
FROM prompt_templates
WHERE tenant_id = $1 AND name = $2
`, tenantID, name).Scan(&nextVersion); err != nil {
		return PromptTemplate{}, fmt.Errorf("resolve next prompt template version: %w", err)
	}
	if nextVersion <= 1 {
		return PromptTemplate{}, sql.ErrNoRows
	}

	outSchemaRaw, _ := json.Marshal(req.OutputSchema)
	metadataRaw, _ := json.Marshal(req.Metadata)
	var item PromptTemplate
	err = tx.QueryRowContext(ctx, `
INSERT INTO prompt_templates (tenant_id, name, version, template, output_schema, metadata, created_by)
VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7)
RETURNING id, tenant_id, name, version, template, COALESCE(output_schema, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb), created_at, created_by
`, tenantID, name, nextVersion, req.Template, string(outSchemaRaw), string(metadataRaw), createdBy).Scan(
		&item.ID,
		&item.TenantID,
		&item.Name,
		&item.Version,
		&item.Template,
		&item.OutputSchema,
		&item.Metadata,
		&item.CreatedAt,
		&item.CreatedBy,
	)
	if err != nil {
		return PromptTemplate{}, fmt.Errorf("insert prompt template version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PromptTemplate{}, fmt.Errorf("commit prompt template version tx: %w", err)
	}
	return item, nil
}

func (s *PromptTemplateService) resolveTemplate(ctx context.Context, tenantID uuid.UUID, raw string, explicitVersion int) (PromptTemplate, error) {
	name, version := parseTemplateReference(raw)
	if explicitVersion > 0 {
		version = explicitVersion
	}
	if name == "" {
		return PromptTemplate{}, fmt.Errorf("prompt template is required")
	}
	if version <= 0 {
		var tpl PromptTemplate
		err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, version, template, COALESCE(output_schema, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb), created_at, created_by
FROM prompt_templates
WHERE tenant_id = $1 AND name = $2
ORDER BY version DESC
LIMIT 1
`, tenantID, name).Scan(
			&tpl.ID,
			&tpl.TenantID,
			&tpl.Name,
			&tpl.Version,
			&tpl.Template,
			&tpl.OutputSchema,
			&tpl.Metadata,
			&tpl.CreatedAt,
			&tpl.CreatedBy,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return PromptTemplate{}, fmt.Errorf("prompt template %q not found", name)
			}
			return PromptTemplate{}, fmt.Errorf("resolve latest prompt template: %w", err)
		}
		return tpl, nil
	}
	tpl, err := s.GetVersion(ctx, tenantID, name, version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PromptTemplate{}, fmt.Errorf("prompt template %q version %d not found", name, version)
		}
		return PromptTemplate{}, err
	}
	return tpl, nil
}

func parseTemplateReference(raw string) (name string, version int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0
	}
	re := regexp.MustCompile(`^(.*)_v([0-9]+)$`)
	parts := re.FindStringSubmatch(raw)
	if len(parts) == 3 {
		v, err := strconv.Atoi(parts[2])
		if err == nil {
			return parts[1], v
		}
	}
	return raw, 0
}
