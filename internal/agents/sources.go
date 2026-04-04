package agents

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func (a *AgentExecutor) queryKnowledge(ctx context.Context, tenantID uuid.UUID, query string, collection string, topK int) ([]KnowledgeResult, error) {
	if strings.TrimSpace(query) == "" {
		return []KnowledgeResult{}, nil
	}
	embedding, err := a.llm.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed knowledge query: %w", err)
	}
	vec := vectorLiteral(embedding)

	filterCollection := strings.TrimSpace(collection)
	querySQL := `
SELECT id, filename, COALESCE(extracted_text, ''), 1 - (embedding <=> $2::vector) AS similarity
FROM vault_documents
WHERE tenant_id = $1
  AND deleted_at IS NULL
  AND embedding IS NOT NULL
`
	args := []any{tenantID, vec}
	if filterCollection != "" {
		querySQL += ` AND COALESCE(metadata->>'collection','') = $3`
		args = append(args, filterCollection)
		querySQL += ` ORDER BY embedding <=> $2::vector LIMIT $4`
		args = append(args, topK)
	} else {
		querySQL += ` ORDER BY embedding <=> $2::vector LIMIT $3`
		args = append(args, topK)
	}

	rows, err := a.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query knowledge documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]KnowledgeResult, 0)
	for rows.Next() {
		var item KnowledgeResult
		if err := rows.Scan(&item.DocumentID, &item.Filename, &item.Text, &item.Similarity); err != nil {
			return nil, fmt.Errorf("scan knowledge result: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge results: %w", err)
	}
	return results, nil
}

func (a *AgentExecutor) queryVaultDocuments(ctx context.Context, tenantID, caseID uuid.UUID, documentTypes []string) ([]VaultDocResult, error) {
	querySQL := `
SELECT id, filename, mime_type, COALESCE(extracted_text,''), COALESCE(metadata, '{}'::jsonb)
FROM vault_documents
WHERE tenant_id = $1
  AND case_id = $2
  AND deleted_at IS NULL
`
	args := []any{tenantID, caseID}
	if len(documentTypes) > 0 {
		querySQL += ` AND COALESCE(metadata->>'document_type','') = ANY($3)`
		args = append(args, toPGTextArray(documentTypes))
	}
	querySQL += ` ORDER BY uploaded_at DESC LIMIT 100`

	rows, err := a.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query vault context documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]VaultDocResult, 0)
	for rows.Next() {
		var item VaultDocResult
		if err := rows.Scan(&item.DocumentID, &item.Filename, &item.MimeType, &item.ExtractedText, &item.Metadata); err != nil {
			return nil, fmt.Errorf("scan vault context document: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vault context documents: %w", err)
	}
	return out, nil
}

func vectorLiteral(v []float32) string {
	parts := make([]string, 0, len(v))
	for _, n := range v {
		parts = append(parts, strconv.FormatFloat(float64(n), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

type pgTextArray []string

func toPGTextArray(items []string) pgTextArray {
	return pgTextArray(items)
}

func (a pgTextArray) Value() (driver.Value, error) {
	quoted := make([]string, len(a))
	for i, v := range a {
		quoted[i] = `"` + strings.ReplaceAll(v, `"`, `\\"`) + `"`
	}
	return "{" + strings.Join(quoted, ",") + "}", nil
}
