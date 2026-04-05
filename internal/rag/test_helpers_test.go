package rag

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
)

type mockEmbedder struct {
	dims int
}

func (m mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		v := []float32{float32(len(t)%10) / 10, float32((len(t)+3)%10) / 10, float32((len(t)+7)%10) / 10}
		out = append(out, v)
	}
	return out, nil
}

func (m mockEmbedder) Dimensions() int   { return m.dims }
func (m mockEmbedder) ModelName() string { return "mock-embed" }

type mockVectorStore struct {
	mu      sync.Mutex
	chunks  map[string][]StorableChunk
	deleted map[string]bool
	search  []SearchResult
}

func newMockVectorStore() *mockVectorStore {
	return &mockVectorStore{chunks: map[string][]StorableChunk{}, deleted: map[string]bool{}}
}

func (m *mockVectorStore) Store(_ context.Context, tenantID string, kbID string, chunks []StorableChunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantID + ":" + kbID
	m.chunks[key] = append([]StorableChunk{}, chunks...)
	return nil
}

func (m *mockVectorStore) Search(_ context.Context, _ []float32, _ SearchOpts) ([]SearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]SearchResult{}, m.search...), nil
}

func (m *mockVectorStore) Delete(_ context.Context, documentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted[documentID] = true
	return nil
}

func (m *mockVectorStore) DeleteAll(_ context.Context, kbID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.chunks {
		if len(k) >= len(kbID) && k[len(k)-len(kbID):] == kbID {
			delete(m.chunks, k)
		}
	}
	return nil
}

func (m *mockVectorStore) Count(_ context.Context, kbID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.chunks {
		if len(k) >= len(kbID) && k[len(k)-len(kbID):] == kbID {
			return len(v), nil
		}
	}
	return 0, nil
}

func (m *mockVectorStore) FullTextSearch(_ context.Context, _ string, _ SearchOpts) ([]SearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]SearchResult{}, m.search...), nil
}

func (m *mockVectorStore) HybridSearch(_ context.Context, _ []float32, _ string, _ SearchOpts) ([]SearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]SearchResult{}, m.search...), nil
}

type mockKBStore struct {
	item KnowledgeBase
}

func (m *mockKBStore) List(_ context.Context, tenantID uuid.UUID) ([]KnowledgeBase, error) {
	if m.item.TenantID == tenantID {
		return []KnowledgeBase{m.item}, nil
	}
	return []KnowledgeBase{}, nil
}

func (m *mockKBStore) Create(_ context.Context, kb KnowledgeBase) (KnowledgeBase, error) {
	m.item = kb
	return kb, nil
}
func (m *mockKBStore) Get(_ context.Context, tenantID, kbID uuid.UUID) (KnowledgeBase, error) {
	if m.item.ID == kbID && (tenantID == uuid.Nil || m.item.TenantID == tenantID) {
		return m.item, nil
	}
	return KnowledgeBase{}, ErrKnowledgeBaseNotFound
}
func (m *mockKBStore) Update(_ context.Context, kb KnowledgeBase) (KnowledgeBase, error) {
	m.item = kb
	return kb, nil
}
func (m *mockKBStore) Delete(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockKBStore) SetStatus(_ context.Context, _, _ uuid.UUID, status string) error {
	m.item.Status = status
	return nil
}
func (m *mockKBStore) RecalculateCounts(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockKBStore) HasMismatchedEmbeddingModel(_ context.Context, _, _ uuid.UUID, model string) (bool, error) {
	if model == "mismatch" {
		return true, nil
	}
	return false, nil
}

type mockDocStore struct {
	mu  sync.Mutex
	doc KnowledgeDocument
}

func (m *mockDocStore) List(_ context.Context, _, _ uuid.UUID) ([]KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return []KnowledgeDocument{m.doc}, nil
}
func (m *mockDocStore) Create(_ context.Context, doc KnowledgeDocument) (KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doc = doc
	return doc, nil
}
func (m *mockDocStore) Get(_ context.Context, _, _, docID uuid.UUID) (KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.doc.ID == docID {
		return m.doc, nil
	}
	return KnowledgeDocument{}, ErrKnowledgeDocumentNotFound
}
func (m *mockDocStore) SetStatus(_ context.Context, _, _, _ uuid.UUID, status string, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doc.Status = status
	m.doc.ErrorMessage = errMsg
	return nil
}
func (m *mockDocStore) SetReady(_ context.Context, _, _, _ uuid.UUID, chunkCount int, processingMS int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doc.Status = "ready"
	m.doc.ChunkCount = chunkCount
	m.doc.ProcessingMS = processingMS
	return nil
}
func (m *mockDocStore) Delete(_ context.Context, _, _, _ uuid.UUID) error { return nil }
func (m *mockDocStore) ListByKB(_ context.Context, _, _ uuid.UUID) ([]KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return []KnowledgeDocument{m.doc}, nil
}
func (m *mockDocStore) ClaimPending(_ context.Context) (KnowledgeDocument, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.doc.Status != "pending" {
		return KnowledgeDocument{}, false, nil
	}
	m.doc.Status = "extracting"
	return m.doc, true, nil
}
func (m *mockDocStore) ResetToPendingByKB(_ context.Context, _, _ uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doc.Status = "pending"
	return nil
}

func (m *mockDocStore) status() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doc.Status
}

type mockVaultStore struct {
	data map[string][]byte
}

func (m *mockVaultStore) Put(_, _, _ string, _ []byte) (string, error) {
	return "", errors.New("not implemented")
}
func (m *mockVaultStore) Get(uri string) ([]byte, error) {
	if b, ok := m.data[uri]; ok {
		return b, nil
	}
	return nil, errors.New("not found")
}
func (m *mockVaultStore) Delete(_ string) error                     { return nil }
func (m *mockVaultStore) SignedURL(_ string, _ int) (string, error) { return "", nil }
