package channels

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type CaseTypeSchemaStore struct {
	db *sql.DB
}

func NewCaseTypeSchemaStore(db *sql.DB) *CaseTypeSchemaStore {
	return &CaseTypeSchemaStore{db: db}
}

func (s *CaseTypeSchemaStore) GetFormSchema(ctx context.Context, tenantID, caseTypeID uuid.UUID) (json.RawMessage, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("case type store unavailable")
	}
	var raw json.RawMessage
	if err := s.db.QueryRowContext(ctx, `
SELECT schema
FROM case_types
WHERE tenant_id = $1 AND id = $2
ORDER BY version DESC
LIMIT 1
`, tenantID, caseTypeID).Scan(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}
