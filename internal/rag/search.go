package rag

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type SearchService struct {
	store    VectorStore
	embedder Embedder
	kbStore  KnowledgeBaseStore
}

func NewSearchService(store VectorStore, embedder Embedder, kbStore KnowledgeBaseStore) *SearchService {
	return &SearchService{store: store, embedder: embedder, kbStore: kbStore}
}

type SearchRequest struct {
	TenantID        uuid.UUID `json:"tenant_id"`
	KnowledgeBaseID uuid.UUID `json:"knowledge_base_id"`
	Query           string    `json:"query"`
	TopK            int       `json:"top_k"`
	MinScore        float64   `json:"min_score"`
	Mode            string    `json:"mode"`
}

type SearchResponse struct {
	Results         []SearchResult `json:"results"`
	Mode            string         `json:"mode"`
	QueryDurationMS int            `json:"query_duration_ms"`
}

func (ss *SearchService) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	mode := req.Mode
	if mode == "" {
		mode = "hybrid"
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.MinScore <= 0 {
		req.MinScore = 0.7
	}
	start := time.Now()

	_, err := ss.kbStore.Get(ctx, req.TenantID, req.KnowledgeBaseID)
	if err != nil {
		return SearchResponse{}, err
	}

	opts := SearchOpts{
		TenantID:  req.TenantID.String(),
		KBID:      req.KnowledgeBaseID.String(),
		TopK:      req.TopK,
		MinScore:  req.MinScore,
		Mode:      mode,
		QueryText: req.Query,
	}

	var results []SearchResult
	switch mode {
	case "fulltext":
		fts, ok := ss.store.(FullTextSearcher)
		if ok {
			results, err = fts.FullTextSearch(ctx, req.Query, opts)
			if err != nil {
				return SearchResponse{}, err
			}
			break
		}
		mode = "vector"
		vectors, embedErr := ss.embedder.Embed(ctx, []string{req.Query})
		if embedErr != nil {
			return SearchResponse{}, fmt.Errorf("embed query: %w", embedErr)
		}
		if len(vectors) != 1 {
			return SearchResponse{}, fmt.Errorf("embedder returned %d vectors, expected 1", len(vectors))
		}
		results, err = ss.store.Search(ctx, vectors[0], opts)
		if err != nil {
			return SearchResponse{}, err
		}
	case "vector":
		vectors, embedErr := ss.embedder.Embed(ctx, []string{req.Query})
		if embedErr != nil {
			return SearchResponse{}, fmt.Errorf("embed query: %w", embedErr)
		}
		if len(vectors) != 1 {
			return SearchResponse{}, fmt.Errorf("embedder returned %d vectors, expected 1", len(vectors))
		}
		results, err = ss.store.Search(ctx, vectors[0], opts)
		if err != nil {
			return SearchResponse{}, err
		}
	case "hybrid":
		vectors, embedErr := ss.embedder.Embed(ctx, []string{req.Query})
		if embedErr != nil {
			return SearchResponse{}, fmt.Errorf("embed query: %w", embedErr)
		}
		if len(vectors) != 1 {
			return SearchResponse{}, fmt.Errorf("embedder returned %d vectors, expected 1", len(vectors))
		}
		if hybrid, ok := ss.store.(HybridSearcher); ok {
			results, err = hybrid.HybridSearch(ctx, vectors[0], req.Query, opts)
			if err != nil {
				return SearchResponse{}, err
			}
		} else {
			results, err = ss.store.Search(ctx, vectors[0], opts)
			if err != nil {
				return SearchResponse{}, err
			}
			mode = "vector"
		}
	default:
		return SearchResponse{}, fmt.Errorf("unsupported search mode %q", mode)
	}

	return SearchResponse{
		Results:         results,
		Mode:            mode,
		QueryDurationMS: int(time.Since(start).Milliseconds()),
	}, nil
}
