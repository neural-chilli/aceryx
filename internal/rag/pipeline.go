package rag

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/vault"
)

type IngestionPipeline struct {
	loader    DocumentLoader
	splitter  TextSplitter
	embedder  Embedder
	store     VectorStore
	kbStore   KnowledgeBaseStore
	docStore  DocumentStore
	vault     vault.VaultStore
	batchSize int
}

func NewIngestionPipeline(loader DocumentLoader, splitter TextSplitter, embedder Embedder, store VectorStore, kbStore KnowledgeBaseStore, docStore DocumentStore, vaultStore vault.VaultStore) *IngestionPipeline {
	return &IngestionPipeline{
		loader:    loader,
		splitter:  splitter,
		embedder:  embedder,
		store:     store,
		kbStore:   kbStore,
		docStore:  docStore,
		vault:     vaultStore,
		batchSize: 50,
	}
}

func (p *IngestionPipeline) ProcessDocument(ctx context.Context, kbID, docID uuid.UUID) error {
	started := time.Now()
	doc, err := p.docStore.Get(ctx, uuid.Nil, kbID, docID)
	if err != nil {
		return err
	}
	kb, err := p.kbStore.Get(ctx, doc.TenantID, kbID)
	if err != nil {
		return err
	}
	if err := p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "extracting", ""); err != nil {
		return err
	}

	blob, err := p.vault.Get(doc.StorageURI)
	if err != nil {
		_ = p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "error", fmt.Sprintf("vault read failed: %v", err))
		return fmt.Errorf("read vault blob: %w", err)
	}
	content, err := p.loader.Load(blob, doc.ContentType)
	if err != nil {
		_ = p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "error", err.Error())
		return fmt.Errorf("extract document text: %w", err)
	}

	if err := p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "chunking", ""); err != nil {
		return err
	}
	chunks, err := p.splitter.Split(content, SplitOpts{
		Strategy:     kb.ChunkingStrategy,
		ChunkSize:    kb.ChunkSize,
		ChunkOverlap: kb.ChunkOverlap,
	})
	if err != nil {
		_ = p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "error", err.Error())
		return fmt.Errorf("split text: %w", err)
	}

	if err := p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "embedding", ""); err != nil {
		return err
	}
	vectors, err := p.embedChunks(ctx, chunks)
	if err != nil {
		_ = p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "error", err.Error())
		return fmt.Errorf("embed chunks: %w", err)
	}

	storable := make([]StorableChunk, 0, len(chunks))
	for i := range chunks {
		id := chunkID(docID, chunks[i].Metadata.ChunkIndex, chunks[i].Content)
		storable = append(storable, StorableChunk{
			ID:         id.String(),
			DocumentID: docID.String(),
			Content:    chunks[i].Content,
			TokenCount: chunks[i].TokenCount,
			Metadata:   chunks[i].Metadata,
			Embedding:  vectors[i],
			Model:      p.embedder.ModelName(),
		})
	}
	if err := p.store.Delete(ctx, docID.String()); err != nil {
		_ = p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "error", err.Error())
		return fmt.Errorf("delete old chunks: %w", err)
	}
	if err := p.store.Store(ctx, doc.TenantID.String(), kbID.String(), storable); err != nil {
		_ = p.docStore.SetStatus(ctx, doc.TenantID, kbID, docID, "error", err.Error())
		return fmt.Errorf("store chunks: %w", err)
	}

	elapsed := int(time.Since(started).Milliseconds())
	if err := p.docStore.SetReady(ctx, doc.TenantID, kbID, docID, len(storable), elapsed); err != nil {
		return err
	}
	if err := p.kbStore.RecalculateCounts(ctx, doc.TenantID, kbID); err != nil {
		return err
	}
	return nil
}

func (p *IngestionPipeline) ReIndex(ctx context.Context, kbID uuid.UUID) error {
	kb, err := p.kbStore.Get(ctx, uuid.Nil, kbID)
	if err != nil {
		return err
	}
	if err := p.kbStore.SetStatus(ctx, kb.TenantID, kbID, "indexing"); err != nil {
		return err
	}
	defer func() {
		_ = p.kbStore.SetStatus(context.Background(), kb.TenantID, kbID, "active")
	}()

	docs, err := p.docStore.ListByKB(ctx, kb.TenantID, kbID)
	if err != nil {
		return err
	}
	if err := p.store.DeleteAll(ctx, kbID.String()); err != nil {
		return err
	}
	for _, d := range docs {
		if err := p.docStore.SetStatus(ctx, kb.TenantID, kbID, d.ID, "pending", ""); err != nil {
			return err
		}
		if err := p.ProcessDocument(ctx, kbID, d.ID); err != nil {
			return err
		}
	}
	if err := p.kbStore.SetStatus(ctx, kb.TenantID, kbID, "active"); err != nil {
		return err
	}
	return p.kbStore.RecalculateCounts(ctx, kb.TenantID, kbID)
}

func EstimateReIndexCost(kb *KnowledgeBase, modelPricing ModelPricing) CostEstimate {
	if kb == nil {
		return CostEstimate{}
	}
	tokens := int64(kb.ChunkCount * max(1, kb.ChunkSize))
	cost := (float64(tokens) / 1_000_000.0) * modelPricing.InputPer1MTokensUSD
	return CostEstimate{
		TotalChunks:      kb.ChunkCount,
		EstimatedTokens:  tokens,
		EstimatedCostUSD: cost,
	}
}

func (p *IngestionPipeline) embedChunks(ctx context.Context, chunks []Chunk) ([][]float32, error) {
	if len(chunks) == 0 {
		return [][]float32{}, nil
	}
	batchSize := p.batchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	all := make([][]float32, 0, len(chunks))
	for i := 0; i < len(chunks); i += batchSize {
		end := min(len(chunks), i+batchSize)
		texts := make([]string, 0, end-i)
		for _, ch := range chunks[i:end] {
			texts = append(texts, ch.Content)
		}
		vecs, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return nil, err
		}
		if len(vecs) != len(texts) {
			return nil, fmt.Errorf("embedder returned %d vectors for %d texts", len(vecs), len(texts))
		}
		all = append(all, vecs...)
	}
	return all, nil
}

func chunkID(documentID uuid.UUID, chunkIndex int, content string) uuid.UUID {
	h := sha256.New()
	h.Write([]byte(documentID.String()))
	_ = binary.Write(h, binary.BigEndian, int64(chunkIndex))
	h.Write([]byte(strings.TrimSpace(content)))
	return uuid.NewSHA1(uuid.NameSpaceOID, h.Sum(nil))
}

func UploadExt(filename string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if ext == "" {
		return "bin"
	}
	return ext
}
