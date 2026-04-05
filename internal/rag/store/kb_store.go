package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/rag"
)

type KnowledgeBaseStore struct {
	db *sql.DB
}

func NewKnowledgeBaseStore(db *sql.DB) *KnowledgeBaseStore {
	return &KnowledgeBaseStore{db: db}
}

func (s *KnowledgeBaseStore) List(ctx context.Context, tenantID uuid.UUID) ([]rag.KnowledgeBase, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, name, COALESCE(description,''), chunking_strategy, chunk_size, chunk_overlap,
       embedding_provider, embedding_model, embedding_dims, vector_store,
       COALESCE(vector_store_config, '{}'::jsonb), document_count, chunk_count, status, created_at, updated_at
FROM knowledge_bases
WHERE tenant_id = $1
ORDER BY created_at DESC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]rag.KnowledgeBase, 0)
	for rows.Next() {
		kb, scanErr := scanKnowledgeBase(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, kb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge bases: %w", err)
	}
	return out, nil
}

func (s *KnowledgeBaseStore) Create(ctx context.Context, kb rag.KnowledgeBase) (rag.KnowledgeBase, error) {
	cfgRaw, _ := json.Marshal(kb.VectorStoreConfig)
	row := s.db.QueryRowContext(ctx, `
INSERT INTO knowledge_bases (
    tenant_id, name, description, chunking_strategy, chunk_size, chunk_overlap,
    embedding_provider, embedding_model, embedding_dims, vector_store, vector_store_config
) VALUES (
    $1, $2, NULLIF($3, ''), $4, $5, $6,
    $7, $8, $9, $10, $11::jsonb
)
RETURNING id, tenant_id, name, COALESCE(description,''), chunking_strategy, chunk_size, chunk_overlap,
          embedding_provider, embedding_model, embedding_dims, vector_store,
          COALESCE(vector_store_config, '{}'::jsonb), document_count, chunk_count, status, created_at, updated_at
`, kb.TenantID, kb.Name, kb.Description, kb.ChunkingStrategy, kb.ChunkSize, kb.ChunkOverlap,
		kb.EmbeddingProvider, kb.EmbeddingModel, kb.EmbeddingDims, kb.VectorStore, string(cfgRaw))
	created, err := scanKnowledgeBase(row)
	if err != nil {
		return rag.KnowledgeBase{}, err
	}
	return created, nil
}

func (s *KnowledgeBaseStore) Get(ctx context.Context, tenantID, kbID uuid.UUID) (rag.KnowledgeBase, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, COALESCE(description,''), chunking_strategy, chunk_size, chunk_overlap,
       embedding_provider, embedding_model, embedding_dims, vector_store,
       COALESCE(vector_store_config, '{}'::jsonb), document_count, chunk_count, status, created_at, updated_at
FROM knowledge_bases
WHERE tenant_id = $1 AND id = $2
`, tenantID, kbID)
	kb, err := scanKnowledgeBase(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rag.KnowledgeBase{}, rag.ErrKnowledgeBaseNotFound
		}
		return rag.KnowledgeBase{}, err
	}
	return kb, nil
}

func (s *KnowledgeBaseStore) Update(ctx context.Context, kb rag.KnowledgeBase) (rag.KnowledgeBase, error) {
	cfgRaw, _ := json.Marshal(kb.VectorStoreConfig)
	row := s.db.QueryRowContext(ctx, `
UPDATE knowledge_bases
SET name = $3,
    description = NULLIF($4, ''),
    chunking_strategy = $5,
    chunk_size = $6,
    chunk_overlap = $7,
    embedding_provider = $8,
    embedding_model = $9,
    embedding_dims = $10,
    vector_store = $11,
    vector_store_config = $12::jsonb,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2
RETURNING id, tenant_id, name, COALESCE(description,''), chunking_strategy, chunk_size, chunk_overlap,
          embedding_provider, embedding_model, embedding_dims, vector_store,
          COALESCE(vector_store_config, '{}'::jsonb), document_count, chunk_count, status, created_at, updated_at
`, kb.TenantID, kb.ID, kb.Name, kb.Description, kb.ChunkingStrategy, kb.ChunkSize, kb.ChunkOverlap,
		kb.EmbeddingProvider, kb.EmbeddingModel, kb.EmbeddingDims, kb.VectorStore, string(cfgRaw))
	updated, err := scanKnowledgeBase(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rag.KnowledgeBase{}, rag.ErrKnowledgeBaseNotFound
		}
		return rag.KnowledgeBase{}, err
	}
	return updated, nil
}

func (s *KnowledgeBaseStore) Delete(ctx context.Context, tenantID, kbID uuid.UUID) error {
	cmd, err := s.db.ExecContext(ctx, `DELETE FROM knowledge_bases WHERE tenant_id = $1 AND id = $2`, tenantID, kbID)
	if err != nil {
		return fmt.Errorf("delete knowledge base: %w", err)
	}
	if n, _ := cmd.RowsAffected(); n == 0 {
		return rag.ErrKnowledgeBaseNotFound
	}
	return nil
}

func (s *KnowledgeBaseStore) SetStatus(ctx context.Context, tenantID, kbID uuid.UUID, status string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_bases
SET status = $3, updated_at = now()
WHERE tenant_id = $1 AND id = $2
`, tenantID, kbID, status)
	if err != nil {
		return fmt.Errorf("set knowledge base status: %w", err)
	}
	return nil
}

func (s *KnowledgeBaseStore) RecalculateCounts(ctx context.Context, tenantID, kbID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_bases kb
SET document_count = COALESCE((
        SELECT COUNT(*)
        FROM knowledge_documents kd
        WHERE kd.knowledge_base_id = kb.id
    ), 0),
    chunk_count = COALESCE((
        SELECT COUNT(*)
        FROM document_chunks dc
        WHERE dc.knowledge_base_id = kb.id AND dc.tenant_id = kb.tenant_id
    ), 0),
    updated_at = now()
WHERE kb.tenant_id = $1 AND kb.id = $2
`, tenantID, kbID)
	if err != nil {
		return fmt.Errorf("recalculate knowledge base counts: %w", err)
	}
	return nil
}

func (s *KnowledgeBaseStore) HasMismatchedEmbeddingModel(ctx context.Context, tenantID, kbID uuid.UUID, model string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM document_chunks dc
    WHERE dc.tenant_id = $1
      AND dc.knowledge_base_id = $2
      AND dc.embedding_model <> $3
)
`, tenantID, kbID, model).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check embedding model compatibility: %w", err)
	}
	return exists, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanKnowledgeBase(row scanner) (rag.KnowledgeBase, error) {
	var kb rag.KnowledgeBase
	var cfgRaw []byte
	if err := row.Scan(
		&kb.ID,
		&kb.TenantID,
		&kb.Name,
		&kb.Description,
		&kb.ChunkingStrategy,
		&kb.ChunkSize,
		&kb.ChunkOverlap,
		&kb.EmbeddingProvider,
		&kb.EmbeddingModel,
		&kb.EmbeddingDims,
		&kb.VectorStore,
		&cfgRaw,
		&kb.DocumentCount,
		&kb.ChunkCount,
		&kb.Status,
		&kb.CreatedAt,
		&kb.UpdatedAt,
	); err != nil {
		return rag.KnowledgeBase{}, err
	}
	kb.VectorStoreConfig = map[string]any{}
	if len(cfgRaw) > 0 {
		_ = json.Unmarshal(cfgRaw, &kb.VectorStoreConfig)
	}
	return kb, nil
}
