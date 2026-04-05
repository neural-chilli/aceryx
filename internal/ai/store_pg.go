package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, tenantID uuid.UUID, def *AIComponentDef, createdBy uuid.UUID) error {
	if s == nil || s.db == nil {
		return nil
	}
	if def == nil {
		return fmt.Errorf("component definition is nil")
	}
	if err := ValidateComponentDef(def); err != nil {
		return err
	}
	body, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("marshal component definition: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO tenant_ai_components (tenant_id, definition, created_by)
VALUES ($1, $2::jsonb, $3)
`, tenantID, string(body), createdBy)
	if err != nil {
		return fmt.Errorf("insert tenant ai component: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, tenantID uuid.UUID, def *AIComponentDef) error {
	if s == nil || s.db == nil {
		return nil
	}
	if def == nil {
		return fmt.Errorf("component definition is nil")
	}
	if err := ValidateComponentDef(def); err != nil {
		return err
	}
	body, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("marshal component definition: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE tenant_ai_components
SET definition = $3::jsonb,
    updated_at = now()
WHERE tenant_id = $1
  AND definition->>'id' = $2
`, tenantID, def.ID, string(body))
	if err != nil {
		return fmt.Errorf("update tenant ai component: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, tenantID uuid.UUID, componentID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM tenant_ai_components
WHERE tenant_id = $1
  AND definition->>'id' = $2
`, tenantID, strings.TrimSpace(componentID))
	if err != nil {
		return fmt.Errorf("delete tenant ai component: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*AIComponentDef, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT definition
FROM tenant_ai_components
WHERE tenant_id = $1
ORDER BY created_at ASC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query tenant ai components: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*AIComponentDef, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan tenant ai component: %w", err)
		}
		var def AIComponentDef
		if err := json.Unmarshal(raw, &def); err != nil {
			return nil, fmt.Errorf("decode tenant ai component: %w", err)
		}
		if err := ValidateComponentDef(&def); err != nil {
			return nil, fmt.Errorf("invalid stored tenant ai component %q: %w", def.ID, err)
		}
		out = append(out, cloneComponent(&def))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenant ai components: %w", err)
	}
	return out, nil
}
