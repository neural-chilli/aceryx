package cases

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

func resolveLatestActiveCaseTypeTx(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, name string) (CaseType, error) {
	var ct CaseType
	var raw []byte
	err := tx.QueryRowContext(ctx, `
SELECT id, tenant_id, name, version, schema, status, created_at, created_by
FROM case_types
WHERE tenant_id = $1 AND name = $2 AND status = 'active'
ORDER BY version DESC
LIMIT 1
`, tenantID, name).Scan(&ct.ID, &ct.TenantID, &ct.Name, &ct.Version, &raw, &ct.Status, &ct.CreatedAt, &ct.CreatedBy)
	if err != nil {
		return CaseType{}, err
	}
	if err := json.Unmarshal(raw, &ct.Schema); err != nil {
		return CaseType{}, err
	}
	return ct, nil
}

func resolveLatestPublishedWorkflowTx(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, caseType string) (uuid.UUID, int, []byte, error) {
	var workflowID uuid.UUID
	var version int
	var ast []byte
	err := tx.QueryRowContext(ctx, `
SELECT w.id, wv.version, wv.ast
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.tenant_id = $1
  AND w.case_type = $2
  AND wv.status = 'published'
ORDER BY wv.version DESC
LIMIT 1
`, tenantID, caseType).Scan(&workflowID, &version, &ast)
	if err != nil {
		return uuid.Nil, 0, nil, err
	}
	return workflowID, version, ast, nil
}

func parseStepIDs(astRaw []byte) ([]string, error) {
	var ast struct {
		Steps []struct {
			ID string `json:"id"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(astRaw, &ast); err != nil {
		return nil, fmt.Errorf("decode workflow ast: %w", err)
	}
	out := make([]string, 0, len(ast.Steps))
	for _, st := range ast.Steps {
		if st.ID != "" {
			out = append(out, st.ID)
		}
	}
	return out, nil
}

func generateCaseNumberTx(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, caseType string) (string, error) {
	prefix := formatCasePrefix(caseType)
	var next int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO case_number_sequences (tenant_id, case_type, last_number)
VALUES ($1, $2, 1)
ON CONFLICT (tenant_id, case_type)
DO UPDATE SET last_number = case_number_sequences.last_number + 1
RETURNING last_number
`, tenantID, caseType).Scan(&next)
	if err != nil {
		return "", fmt.Errorf("generate case number sequence: %w", err)
	}
	return fmt.Sprintf("%s-%06d", prefix, next), nil
}

func formatCasePrefix(caseType string) string {
	parts := strings.Split(caseType, "_")
	if len(parts) == 1 {
		parts = strings.Split(caseType, "-")
	}
	if len(parts) == 1 {
		parts = strings.Fields(caseType)
	}
	prefix := ""
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		r := []rune(strings.ToUpper(p))
		prefix += string(r[0])
	}
	if prefix == "" {
		clean := regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(strings.ToUpper(caseType), "")
		if len(clean) >= 4 {
			prefix = clean[:4]
		} else {
			prefix = clean
		}
	}
	if len(prefix) < 2 {
		prefix = strings.ToUpper(caseType)
	}
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return prefix
}
