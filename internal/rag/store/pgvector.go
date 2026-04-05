package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neural-chilli/aceryx/internal/rag"
)

type PgVectorStore struct {
	db *pgxpool.Pool
}

func NewPgVectorStore(db *pgxpool.Pool) *PgVectorStore {
	return &PgVectorStore{db: db}
}

func (s *PgVectorStore) Store(ctx context.Context, tenantID, kbID string, chunks []rag.StorableChunk) error {
	if s == nil || s.db == nil || len(chunks) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, ch := range chunks {
		metaRaw, _ := json.Marshal(ch.Metadata)
		batch.Queue(`
INSERT INTO document_chunks (
    id, tenant_id, knowledge_base_id, document_id, content, token_count, metadata, embedding, embedding_model
) VALUES (
    $1, $2, $3, $4, $5, $6, $7::jsonb, $8::vector, $9
)
ON CONFLICT (id)
DO UPDATE SET
    content = EXCLUDED.content,
    token_count = EXCLUDED.token_count,
    metadata = EXCLUDED.metadata,
    embedding = EXCLUDED.embedding,
    embedding_model = EXCLUDED.embedding_model
`, ch.ID, tenantID, kbID, ch.DocumentID, ch.Content, ch.TokenCount, string(metaRaw), vectorLiteral(ch.Embedding), ch.Model)
	}
	results := s.db.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()
	for range chunks {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("store chunk batch: %w", err)
		}
	}
	return nil
}

func (s *PgVectorStore) Search(ctx context.Context, query []float32, opts rag.SearchOpts) ([]rag.SearchResult, error) {
	topK := opts.TopK
	if topK <= 0 {
		topK = 5
	}
	minScore := opts.MinScore
	if minScore <= 0 {
		minScore = 0.7
	}
	vec := vectorLiteral(query)
	rows, err := s.db.Query(ctx, `
SELECT id::text, content, metadata, document_id::text,
       1 - (embedding <=> $1::vector) AS score
FROM document_chunks
WHERE tenant_id = $2
  AND knowledge_base_id = $3
  AND 1 - (embedding <=> $1::vector) >= $4
ORDER BY embedding <=> $1::vector
LIMIT $5
`, vec, opts.TenantID, opts.KBID, minScore, topK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()
	return scanSearchResults(rows)
}

func (s *PgVectorStore) FullTextSearch(ctx context.Context, query string, opts rag.SearchOpts) ([]rag.SearchResult, error) {
	topK := opts.TopK
	if topK <= 0 {
		topK = 5
	}
	rows, err := s.db.Query(ctx, `
SELECT id::text, content, metadata, document_id::text,
       ts_rank(content_tsv, plainto_tsquery('english', $1)) AS score
FROM document_chunks
WHERE tenant_id = $2
  AND knowledge_base_id = $3
  AND content_tsv @@ plainto_tsquery('english', $1)
ORDER BY score DESC
LIMIT $4
`, query, opts.TenantID, opts.KBID, topK)
	if err != nil {
		return nil, fmt.Errorf("full-text search: %w", err)
	}
	defer rows.Close()
	return scanSearchResults(rows)
}

func (s *PgVectorStore) HybridSearch(ctx context.Context, query []float32, text string, opts rag.SearchOpts) ([]rag.SearchResult, error) {
	vectorResults, err := s.Search(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	ftsResults, err := s.FullTextSearch(ctx, text, opts)
	if err != nil {
		return nil, err
	}
	return hybridSearch(vectorResults, ftsResults, opts.TopK), nil
}

func (s *PgVectorStore) Delete(ctx context.Context, documentID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("delete chunks by document: %w", err)
	}
	return nil
}

func (s *PgVectorStore) DeleteAll(ctx context.Context, kbID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM document_chunks WHERE knowledge_base_id = $1`, kbID); err != nil {
		return fmt.Errorf("delete chunks by kb: %w", err)
	}
	return nil
}

func (s *PgVectorStore) Count(ctx context.Context, kbID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var count int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM document_chunks WHERE knowledge_base_id = $1`, kbID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count chunks: %w", err)
	}
	return count, nil
}

func hybridSearch(vectorResults, ftsResults []rag.SearchResult, k int) []rag.SearchResult {
	if k <= 0 {
		k = 5
	}
	const rrfK = 60.0
	scores := map[string]float64{}
	items := map[string]rag.SearchResult{}

	for rank, r := range vectorResults {
		scores[r.ChunkID] += 1.0 / (rrfK + float64(rank+1))
		items[r.ChunkID] = r
	}
	for rank, r := range ftsResults {
		scores[r.ChunkID] += 1.0 / (rrfK + float64(rank+1))
		if existing, ok := items[r.ChunkID]; ok {
			if strings.TrimSpace(existing.Content) == "" {
				items[r.ChunkID] = r
			}
			continue
		}
		items[r.ChunkID] = r
	}

	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if scores[keys[i]] == scores[keys[j]] {
			return keys[i] < keys[j]
		}
		return scores[keys[i]] > scores[keys[j]]
	})
	if len(keys) > k {
		keys = keys[:k]
	}
	out := make([]rag.SearchResult, 0, len(keys))
	for _, key := range keys {
		item := items[key]
		item.Score = scores[key]
		out = append(out, item)
	}
	return out
}

func scanSearchResults(rows pgxRows) ([]rag.SearchResult, error) {
	out := make([]rag.SearchResult, 0)
	for rows.Next() {
		var (
			item    rag.SearchResult
			metaRaw []byte
		)
		if err := rows.Scan(&item.ChunkID, &item.Content, &metaRaw, &item.DocumentID, &item.Score); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &item.Metadata)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search rows: %w", err)
	}
	return out, nil
}

type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func vectorLiteral(v []float32) string {
	parts := make([]string, 0, len(v))
	for _, n := range v {
		parts = append(parts, strconv.FormatFloat(float64(n), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
