package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/rag"
)

type DocumentStore struct {
	db *sql.DB
}

func NewDocumentStore(db *sql.DB) *DocumentStore {
	return &DocumentStore{db: db}
}

func (s *DocumentStore) List(ctx context.Context, tenantID, kbID uuid.UUID) ([]rag.KnowledgeDocument, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT kd.id, kd.knowledge_base_id, kd.vault_document_id, kb.tenant_id,
       kd.filename, kd.content_type, COALESCE(kd.file_size, 0), kd.status,
       COALESCE(kd.chunk_count, 0), COALESCE(kd.error_message, ''), COALESCE(kd.processing_ms, 0),
       COALESCE(vd.storage_uri, ''), kd.created_at, kd.updated_at
FROM knowledge_documents kd
JOIN knowledge_bases kb ON kb.id = kd.knowledge_base_id
LEFT JOIN vault_documents vd ON vd.id = kd.vault_document_id
WHERE kb.tenant_id = $1 AND kd.knowledge_base_id = $2
ORDER BY kd.created_at DESC
`, tenantID, kbID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]rag.KnowledgeDocument, 0)
	for rows.Next() {
		doc, scanErr := scanKnowledgeDocument(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}
	return out, nil
}

func (s *DocumentStore) Create(ctx context.Context, doc rag.KnowledgeDocument) (rag.KnowledgeDocument, error) {
	row := s.db.QueryRowContext(ctx, `
INSERT INTO knowledge_documents (
    knowledge_base_id, vault_document_id, filename, content_type, file_size, status
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING id, knowledge_base_id, vault_document_id,
          (SELECT tenant_id FROM knowledge_bases WHERE id = knowledge_base_id),
          filename, content_type, COALESCE(file_size, 0), status,
          COALESCE(chunk_count, 0), COALESCE(error_message, ''), COALESCE(processing_ms, 0),
          COALESCE((SELECT storage_uri FROM vault_documents WHERE id = vault_document_id), ''),
          created_at, updated_at
`, doc.KnowledgeBaseID, doc.VaultDocumentID, doc.Filename, doc.ContentType, doc.FileSize, doc.Status)
	created, err := scanKnowledgeDocument(row)
	if err != nil {
		return rag.KnowledgeDocument{}, fmt.Errorf("create document: %w", err)
	}
	return created, nil
}

func (s *DocumentStore) Get(ctx context.Context, tenantID, kbID, docID uuid.UUID) (rag.KnowledgeDocument, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT kd.id, kd.knowledge_base_id, kd.vault_document_id, kb.tenant_id,
       kd.filename, kd.content_type, COALESCE(kd.file_size, 0), kd.status,
       COALESCE(kd.chunk_count, 0), COALESCE(kd.error_message, ''), COALESCE(kd.processing_ms, 0),
       COALESCE(vd.storage_uri, ''), kd.created_at, kd.updated_at
FROM knowledge_documents kd
JOIN knowledge_bases kb ON kb.id = kd.knowledge_base_id
LEFT JOIN vault_documents vd ON vd.id = kd.vault_document_id
WHERE kb.tenant_id = $1 AND kd.knowledge_base_id = $2 AND kd.id = $3
`, tenantID, kbID, docID)
	doc, err := scanKnowledgeDocument(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rag.KnowledgeDocument{}, rag.ErrKnowledgeDocumentNotFound
		}
		return rag.KnowledgeDocument{}, err
	}
	return doc, nil
}

func (s *DocumentStore) SetStatus(ctx context.Context, tenantID, kbID, docID uuid.UUID, status string, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_documents kd
SET status = $4,
    error_message = NULLIF($5, ''),
    updated_at = now()
FROM knowledge_bases kb
WHERE kd.knowledge_base_id = kb.id
  AND kb.tenant_id = $1
  AND kd.knowledge_base_id = $2
  AND kd.id = $3
`, tenantID, kbID, docID, status, errMsg)
	if err != nil {
		return fmt.Errorf("set document status: %w", err)
	}
	return nil
}

func (s *DocumentStore) SetReady(ctx context.Context, tenantID, kbID, docID uuid.UUID, chunkCount int, processingMS int) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_documents kd
SET status = 'ready',
    chunk_count = $4,
    processing_ms = $5,
    error_message = NULL,
    updated_at = now()
FROM knowledge_bases kb
WHERE kd.knowledge_base_id = kb.id
  AND kb.tenant_id = $1
  AND kd.knowledge_base_id = $2
  AND kd.id = $3
`, tenantID, kbID, docID, chunkCount, processingMS)
	if err != nil {
		return fmt.Errorf("set document ready: %w", err)
	}
	return nil
}

func (s *DocumentStore) Delete(ctx context.Context, tenantID, kbID, docID uuid.UUID) error {
	cmd, err := s.db.ExecContext(ctx, `
DELETE FROM knowledge_documents kd
USING knowledge_bases kb
WHERE kd.knowledge_base_id = kb.id
  AND kb.tenant_id = $1
  AND kd.knowledge_base_id = $2
  AND kd.id = $3
`, tenantID, kbID, docID)
	if err != nil {
		return fmt.Errorf("delete knowledge document: %w", err)
	}
	if n, _ := cmd.RowsAffected(); n == 0 {
		return rag.ErrKnowledgeDocumentNotFound
	}
	return nil
}

func (s *DocumentStore) ListByKB(ctx context.Context, tenantID, kbID uuid.UUID) ([]rag.KnowledgeDocument, error) {
	return s.List(ctx, tenantID, kbID)
}

func (s *DocumentStore) ClaimPending(ctx context.Context) (rag.KnowledgeDocument, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return rag.KnowledgeDocument{}, false, fmt.Errorf("begin claim pending tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
WITH next_doc AS (
    SELECT kd.id
    FROM knowledge_documents kd
    WHERE kd.status = 'pending'
    ORDER BY kd.created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE knowledge_documents kd
SET status = 'extracting',
    error_message = NULL,
    updated_at = now()
FROM next_doc
WHERE kd.id = next_doc.id
RETURNING kd.id, kd.knowledge_base_id
`)

	var docID, kbID uuid.UUID
	if err := row.Scan(&docID, &kbID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if commitErr := tx.Commit(); commitErr != nil {
				return rag.KnowledgeDocument{}, false, fmt.Errorf("commit empty claim tx: %w", commitErr)
			}
			return rag.KnowledgeDocument{}, false, nil
		}
		return rag.KnowledgeDocument{}, false, fmt.Errorf("claim pending document: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return rag.KnowledgeDocument{}, false, fmt.Errorf("commit claim pending tx: %w", err)
	}

	doc, err := s.getByIDs(ctx, kbID, docID)
	if err != nil {
		return rag.KnowledgeDocument{}, false, err
	}
	return doc, true, nil
}

func (s *DocumentStore) ResetToPendingByKB(ctx context.Context, tenantID, kbID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_documents kd
SET status = 'pending',
    chunk_count = 0,
    error_message = NULL,
    processing_ms = NULL,
    updated_at = now()
FROM knowledge_bases kb
WHERE kd.knowledge_base_id = kb.id
  AND kb.tenant_id = $1
  AND kd.knowledge_base_id = $2
`, tenantID, kbID)
	if err != nil {
		return fmt.Errorf("reset documents to pending: %w", err)
	}
	return nil
}

func (s *DocumentStore) getByIDs(ctx context.Context, kbID, docID uuid.UUID) (rag.KnowledgeDocument, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT kd.id, kd.knowledge_base_id, kd.vault_document_id, kb.tenant_id,
       kd.filename, kd.content_type, COALESCE(kd.file_size, 0), kd.status,
       COALESCE(kd.chunk_count, 0), COALESCE(kd.error_message, ''), COALESCE(kd.processing_ms, 0),
       COALESCE(vd.storage_uri, ''), kd.created_at, kd.updated_at
FROM knowledge_documents kd
JOIN knowledge_bases kb ON kb.id = kd.knowledge_base_id
LEFT JOIN vault_documents vd ON vd.id = kd.vault_document_id
WHERE kd.knowledge_base_id = $1 AND kd.id = $2
`, kbID, docID)
	doc, err := scanKnowledgeDocument(row)
	if err != nil {
		return rag.KnowledgeDocument{}, err
	}
	return doc, nil
}

func scanKnowledgeDocument(row scanner) (rag.KnowledgeDocument, error) {
	var doc rag.KnowledgeDocument
	if err := row.Scan(
		&doc.ID,
		&doc.KnowledgeBaseID,
		&doc.VaultDocumentID,
		&doc.TenantID,
		&doc.Filename,
		&doc.ContentType,
		&doc.FileSize,
		&doc.Status,
		&doc.ChunkCount,
		&doc.ErrorMessage,
		&doc.ProcessingMS,
		&doc.StorageURI,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	); err != nil {
		return rag.KnowledgeDocument{}, err
	}
	return doc, nil
}
