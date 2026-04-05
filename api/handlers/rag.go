package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/rag"
	"github.com/neural-chilli/aceryx/internal/vault"
)

type RAGHandlers struct {
	API       *rag.API
	DB        *sql.DB
	Vault     vault.VaultStore
	MaxUpload int64
}

func NewRAGHandlers(api *rag.API, db *sql.DB, vaultStore vault.VaultStore) *RAGHandlers {
	return &RAGHandlers{API: api, DB: db, Vault: vaultStore, MaxUpload: 25 * 1024 * 1024}
}

func (h *RAGHandlers) ListKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := h.API.ListKnowledgeBases(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *RAGHandlers) CreateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req rag.KnowledgeBase
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.TenantID = principal.TenantID
	created, err := h.API.CreateKnowledgeBase(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *RAGHandlers) GetKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	kb, err := h.API.GetKnowledgeBase(r.Context(), principal.TenantID, id)
	if err != nil {
		if errors.Is(err, rag.ErrKnowledgeBaseNotFound) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, kb)
}

func (h *RAGHandlers) UpdateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	var req rag.KnowledgeBase
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.ID = id
	req.TenantID = principal.TenantID
	updated, err := h.API.UpdateKnowledgeBase(r.Context(), req)
	if err != nil {
		if errors.Is(err, rag.ErrKnowledgeBaseNotFound) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *RAGHandlers) DeleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.API.DeleteKnowledgeBase(r.Context(), principal.TenantID, id); err != nil {
		if errors.Is(err, rag.ErrKnowledgeBaseNotFound) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (h *RAGHandlers) ListDocuments(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	kbID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	docs, err := h.API.ListDocuments(r.Context(), principal.TenantID, kbID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *RAGHandlers) UploadDocument(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	kbID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(h.MaxUpload + 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart")
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required")
		return
	}
	defer func() { _ = f.Close() }()
	buf, err := io.ReadAll(io.LimitReader(f, h.MaxUpload+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read_file_failed")
		return
	}
	if int64(len(buf)) > h.MaxUpload {
		writeError(w, http.StatusBadRequest, "file_too_large")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	hash := vault.ContentHash(buf)
	uri, err := h.Vault.Put(principal.TenantID.String(), hash, rag.UploadExt(header.Filename), buf)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	var vaultDocID uuid.UUID
	err = h.DB.QueryRowContext(r.Context(), `
INSERT INTO vault_documents (tenant_id, case_id, step_id, filename, mime_type, size_bytes, content_hash, storage_uri, uploaded_by, metadata)
VALUES ($1, NULL, NULL, $2, $3, $4, $5, $6, $7, '{}'::jsonb)
RETURNING id
`, principal.TenantID, header.Filename, contentType, len(buf), hash, uri, principal.ID).Scan(&vaultDocID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	doc, err := h.API.CreateDocument(r.Context(), principal.TenantID, kbID, rag.KnowledgeDocument{
		VaultDocumentID: vaultDocID,
		Filename:        header.Filename,
		ContentType:     contentType,
		FileSize:        int64(len(buf)),
		Status:          "pending",
	})
	if err != nil {
		if errors.Is(err, rag.ErrUploadsBlocked) {
			writeError(w, http.StatusConflict, "uploads_blocked_stale_embeddings")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *RAGHandlers) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	kbID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	docID, ok := parseUUIDPath(w, r, "doc_id", "invalid_doc_id")
	if !ok {
		return
	}
	if err := h.API.DeleteDocument(r.Context(), principal.TenantID, kbID, docID); err != nil {
		if errors.Is(err, rag.ErrKnowledgeDocumentNotFound) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (h *RAGHandlers) ReIndex(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	kbID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	confirm := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("confirm")), "true")
	estimate, started, err := h.API.ReIndex(r.Context(), principal.TenantID, kbID, confirm)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	if !started {
		writeJSON(w, http.StatusOK, map[string]any{"requires_confirmation": true, "estimate": estimate})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "indexing", "estimate": estimate})
}

func (h *RAGHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	kbID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	stats, err := h.API.Stats(r.Context(), principal.TenantID, kbID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *RAGHandlers) Search(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	kbID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	var req struct {
		Query    string  `json:"query"`
		TopK     int     `json:"top_k"`
		MinScore float64 `json:"min_score"`
		Mode     string  `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	resp, err := h.API.Search(r.Context(), rag.SearchRequest{
		TenantID:        principal.TenantID,
		KnowledgeBaseID: kbID,
		Query:           req.Query,
		TopK:            req.TopK,
		MinScore:        req.MinScore,
		Mode:            req.Mode,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func parseUUIDPath(w http.ResponseWriter, r *http.Request, key, code string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue(key)))
	if err != nil {
		writeError(w, http.StatusBadRequest, code)
		return uuid.Nil, false
	}
	return id, true
}
