package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type API struct {
	KBs              KnowledgeBaseStore
	Docs             DocumentStore
	SearchService    *SearchService
	Pipeline         *IngestionPipeline
	VectorStore      VectorStore
	ReIndexThreshold float64
	ModelPricing     ModelPricing
}

func NewAPI(kbs KnowledgeBaseStore, docs DocumentStore, searchService *SearchService, pipeline *IngestionPipeline, vectorStore VectorStore, modelPricing ModelPricing) *API {
	return &API{
		KBs:              kbs,
		Docs:             docs,
		SearchService:    searchService,
		Pipeline:         pipeline,
		VectorStore:      vectorStore,
		ReIndexThreshold: DefaultReIndexCostGuardUSD,
		ModelPricing:     modelPricing,
	}
}

func (a *API) ListKnowledgeBases(ctx context.Context, tenantID uuid.UUID) ([]KnowledgeBase, error) {
	return a.KBs.List(ctx, tenantID)
}

func (a *API) CreateKnowledgeBase(ctx context.Context, kb KnowledgeBase) (KnowledgeBase, error) {
	if strings.TrimSpace(kb.Name) == "" {
		return KnowledgeBase{}, fmt.Errorf("knowledge base name is required")
	}
	if kb.ChunkingStrategy == "" {
		kb.ChunkingStrategy = "recursive"
	}
	if kb.ChunkSize <= 0 {
		kb.ChunkSize = 512
	}
	if kb.ChunkOverlap < 0 {
		kb.ChunkOverlap = 50
	}
	if kb.EmbeddingProvider == "" {
		kb.EmbeddingProvider = "openai"
	}
	if kb.EmbeddingModel == "" {
		kb.EmbeddingModel = "text-embedding-3-small"
	}
	if kb.EmbeddingDims <= 0 {
		kb.EmbeddingDims = 1536
	}
	if kb.VectorStore == "" {
		kb.VectorStore = "pgvector"
	}
	return a.KBs.Create(ctx, kb)
}

func (a *API) GetKnowledgeBase(ctx context.Context, tenantID, kbID uuid.UUID) (KnowledgeBase, error) {
	return a.KBs.Get(ctx, tenantID, kbID)
}

func (a *API) UpdateKnowledgeBase(ctx context.Context, kb KnowledgeBase) (KnowledgeBase, error) {
	updated, err := a.KBs.Update(ctx, kb)
	if err != nil {
		return KnowledgeBase{}, err
	}
	if err := CheckEmbeddingCompatibility(ctx, &updated, a.KBs); err != nil {
		return updated, err
	}
	return updated, nil
}

func (a *API) DeleteKnowledgeBase(ctx context.Context, tenantID, kbID uuid.UUID) error {
	if err := a.VectorStore.DeleteAll(ctx, kbID.String()); err != nil {
		return err
	}
	return a.KBs.Delete(ctx, tenantID, kbID)
}

func (a *API) ListDocuments(ctx context.Context, tenantID, kbID uuid.UUID) ([]KnowledgeDocument, error) {
	return a.Docs.List(ctx, tenantID, kbID)
}

func (a *API) CreateDocument(ctx context.Context, tenantID, kbID uuid.UUID, doc KnowledgeDocument) (KnowledgeDocument, error) {
	kb, err := a.KBs.Get(ctx, tenantID, kbID)
	if err != nil {
		return KnowledgeDocument{}, err
	}
	if !IsUploadAllowed(&kb) {
		return KnowledgeDocument{}, ErrUploadsBlocked
	}
	doc.KnowledgeBaseID = kbID
	if doc.Status == "" {
		doc.Status = "pending"
	}
	created, err := a.Docs.Create(ctx, doc)
	if err != nil {
		return KnowledgeDocument{}, err
	}
	if err := a.KBs.RecalculateCounts(ctx, tenantID, kbID); err != nil {
		return KnowledgeDocument{}, err
	}
	return created, nil
}

func (a *API) DeleteDocument(ctx context.Context, tenantID, kbID, docID uuid.UUID) error {
	if err := a.VectorStore.Delete(ctx, docID.String()); err != nil {
		return err
	}
	if err := a.Docs.Delete(ctx, tenantID, kbID, docID); err != nil {
		return err
	}
	return a.KBs.RecalculateCounts(ctx, tenantID, kbID)
}

func (a *API) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	return a.SearchService.Search(ctx, req)
}

func (a *API) ReIndex(ctx context.Context, tenantID, kbID uuid.UUID, confirm bool) (CostEstimate, bool, error) {
	kb, err := a.KBs.Get(ctx, tenantID, kbID)
	if err != nil {
		return CostEstimate{}, false, err
	}
	estimate := EstimateReIndexCost(&kb, a.ModelPricing)
	threshold := a.ReIndexThreshold
	if threshold <= 0 {
		threshold = DefaultReIndexCostGuardUSD
	}
	if RequiresReIndexConfirmation(estimate, threshold) && !confirm {
		return estimate, false, nil
	}
	if err := a.Pipeline.ReIndex(ctx, kbID); err != nil {
		return estimate, false, err
	}
	return estimate, true, nil
}

func (a *API) Stats(ctx context.Context, tenantID, kbID uuid.UUID) (KnowledgeBase, error) {
	if err := a.KBs.RecalculateCounts(ctx, tenantID, kbID); err != nil {
		return KnowledgeBase{}, err
	}
	return a.KBs.Get(ctx, tenantID, kbID)
}
