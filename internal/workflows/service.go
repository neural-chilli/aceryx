package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Service struct {
	db      *sql.DB
	catalog aiComponentCatalog
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) SetAIComponentCatalog(catalog aiComponentCatalog) {
	s.catalog = catalog
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]Workflow, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT w.id, w.name, w.case_type,
       COALESCE(wv.version, 0) AS version,
       wv.published_at
FROM workflows w
LEFT JOIN workflow_versions wv
  ON wv.workflow_id = w.id
 AND wv.status = 'published'
WHERE w.tenant_id = $1
ORDER BY w.name ASC, wv.version DESC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type key struct {
		id uuid.UUID
	}
	ordered := make([]Workflow, 0)
	indexByID := map[key]int{}

	for rows.Next() {
		var (
			id          uuid.UUID
			name        string
			caseTypeID  string
			version     int
			publishedAt sql.NullTime
		)
		if err := rows.Scan(&id, &name, &caseTypeID, &version, &publishedAt); err != nil {
			return nil, fmt.Errorf("scan workflow row: %w", err)
		}
		k := key{id: id}
		idx, ok := indexByID[k]
		if !ok {
			ordered = append(ordered, Workflow{
				ID:         id,
				Name:       name,
				CaseTypeID: caseTypeID,
			})
			idx = len(ordered) - 1
			indexByID[k] = idx
		}
		if version > 0 {
			var publishedPtr *time.Time
			if publishedAt.Valid {
				t := publishedAt.Time.UTC()
				publishedPtr = &t
			}
			ordered[idx].PublishedVersions = append(ordered[idx].PublishedVersions, PublishedVersion{
				Version:     version,
				PublishedAt: publishedPtr,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow rows: %w", err)
	}

	return ordered, nil
}

func (s *Service) Create(ctx context.Context, tenantID, actorID uuid.UUID, req CreateRequest) (Workflow, error) {
	name := strings.TrimSpace(req.Name)
	caseTypeID := strings.TrimSpace(req.CaseTypeID)
	if name == "" {
		return Workflow{}, fmt.Errorf("name is required")
	}
	if caseTypeID == "" {
		return Workflow{}, fmt.Errorf("case_type_id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Workflow{}, fmt.Errorf("begin create workflow tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var out Workflow
	err = tx.QueryRowContext(ctx, `
INSERT INTO workflows (tenant_id, name, case_type, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, case_type
`, tenantID, name, caseTypeID, actorID).Scan(&out.ID, &out.Name, &out.CaseTypeID)
	if err != nil {
		return Workflow{}, fmt.Errorf("create workflow: %w", err)
	}

	initialAST := json.RawMessage(`{"steps":[]}`)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO workflow_versions (workflow_id, version, status, ast, yaml_source, created_by)
VALUES ($1, 1, 'draft', $2::jsonb, '', $3)
`, out.ID, string(initialAST), actorID); err != nil {
		return Workflow{}, fmt.Errorf("create initial workflow draft: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Workflow{}, fmt.Errorf("commit create workflow tx: %w", err)
	}
	return out, nil
}

func (s *Service) GetDraftAST(ctx context.Context, tenantID, workflowID uuid.UUID) (json.RawMessage, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT wv.ast
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.id = $1 AND w.tenant_id = $2 AND wv.status = 'draft'
ORDER BY wv.version DESC
LIMIT 1
`, workflowID, tenantID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func (s *Service) SaveDraftAST(ctx context.Context, tenantID, workflowID uuid.UUID, ast json.RawMessage) error {
	if len(ast) == 0 {
		return fmt.Errorf("ast is required")
	}
	var decoded map[string]any
	if err := json.Unmarshal(ast, &decoded); err != nil {
		return fmt.Errorf("invalid ast json: %w", err)
	}
	if err := validateWorkflowAST(ast); err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, `
UPDATE workflow_versions wv
SET ast = $3::jsonb
FROM workflows w
WHERE w.id = $1
  AND w.tenant_id = $2
  AND wv.workflow_id = w.id
  AND wv.status = 'draft'
`, workflowID, tenantID, string(ast))
	if err != nil {
		return fmt.Errorf("save workflow draft ast: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Service) PublishDraft(ctx context.Context, tenantID, workflowID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin publish workflow tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		draftID uuid.UUID
		astRaw  []byte
	)
	err = tx.QueryRowContext(ctx, `
SELECT wv.id, wv.ast
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.id = $1 AND w.tenant_id = $2 AND wv.status = 'draft'
ORDER BY wv.version DESC
LIMIT 1
`, workflowID, tenantID).Scan(&draftID, &astRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var publishedExists bool
			publishedErr := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM workflows w
  JOIN workflow_versions wv ON wv.workflow_id = w.id
  WHERE w.id = $1
    AND w.tenant_id = $2
    AND wv.status = 'published'
)
`, workflowID, tenantID).Scan(&publishedExists)
			if publishedErr != nil {
				return publishedErr
			}
			if publishedExists {
				if err := tx.Commit(); err != nil {
					return fmt.Errorf("commit idempotent publish tx: %w", err)
				}
				return nil
			}
		}
		return err
	}
	if err := validatePublishWorkflow(ctx, tenantID, astRaw, s.catalog); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE workflow_versions wv
SET status = 'withdrawn'
FROM workflows w
WHERE w.id = $1
  AND w.tenant_id = $2
  AND wv.workflow_id = w.id
  AND wv.status = 'published'
`, workflowID, tenantID); err != nil {
		return fmt.Errorf("withdraw previous published versions: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE workflow_versions
SET status = 'published',
    published_at = now()
WHERE id = $1
`, draftID); err != nil {
		return fmt.Errorf("publish draft version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit publish workflow tx: %w", err)
	}
	return nil
}

func (s *Service) ExportYAMLLatest(ctx context.Context, tenantID, workflowID uuid.UUID) (string, error) {
	var (
		yamlSource string
		astRaw     []byte
	)
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(wv.yaml_source, ''), wv.ast
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.id = $1 AND w.tenant_id = $2
ORDER BY CASE wv.status WHEN 'published' THEN 0 WHEN 'draft' THEN 1 ELSE 2 END, wv.version DESC
LIMIT 1
`, workflowID, tenantID).Scan(&yamlSource, &astRaw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(yamlSource) != "" {
		return yamlSource, nil
	}
	return marshalYAMLFromAST(astRaw)
}

func (s *Service) ExportYAMLVersion(ctx context.Context, tenantID, workflowID uuid.UUID, version int) (string, error) {
	var (
		yamlSource string
		astRaw     []byte
	)
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(wv.yaml_source, ''), wv.ast
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.id = $1
  AND w.tenant_id = $2
  AND wv.version = $3
LIMIT 1
`, workflowID, tenantID, version).Scan(&yamlSource, &astRaw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(yamlSource) != "" {
		return yamlSource, nil
	}
	return marshalYAMLFromAST(astRaw)
}

func marshalYAMLFromAST(astRaw []byte) (string, error) {
	var decoded any
	if err := json.Unmarshal(astRaw, &decoded); err != nil {
		return "", fmt.Errorf("decode ast json: %w", err)
	}
	out, err := yaml.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("marshal yaml from ast: %w", err)
	}
	return string(out), nil
}

func (s *Service) ImportYAMLDraft(ctx context.Context, tenantID, workflowID uuid.UUID, yamlSource string) error {
	yamlSource = strings.TrimSpace(yamlSource)
	if yamlSource == "" {
		return fmt.Errorf("yaml is required")
	}
	var decoded any
	if err := yaml.Unmarshal([]byte(yamlSource), &decoded); err != nil {
		return fmt.Errorf("invalid yaml: %w", err)
	}
	astRaw, err := json.Marshal(decoded)
	if err != nil {
		return fmt.Errorf("convert yaml to ast json: %w", err)
	}
	if err := validateWorkflowAST(astRaw); err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, `
UPDATE workflow_versions wv
SET ast = $3::jsonb,
    yaml_source = $4
FROM workflows w
WHERE w.id = $1
  AND w.tenant_id = $2
  AND wv.workflow_id = w.id
  AND wv.status = 'draft'
`, workflowID, tenantID, string(astRaw), yamlSource)
	if err != nil {
		return fmt.Errorf("import workflow yaml into draft: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
