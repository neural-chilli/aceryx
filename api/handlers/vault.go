package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/vault"
)

type VaultHandlers struct {
	Service     *vault.Service
	MaxFileSize int64
}

func NewVaultHandlers(service *vault.Service) *VaultHandlers {
	return &VaultHandlers{Service: service, MaxFileSize: maxDocumentSizeFromEnv()}
}

func (h *VaultHandlers) Upload(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, err := uuid.Parse(r.PathValue("case_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_case_id")
		return
	}
	if err := r.ParseMultipartForm(h.MaxFileSize + 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required")
		return
	}
	defer func() { _ = file.Close() }()

	if header.Size > h.MaxFileSize {
		writeError(w, http.StatusBadRequest, "file_too_large")
		return
	}
	buf, err := io.ReadAll(io.LimitReader(file, h.MaxFileSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read_file_failed")
		return
	}
	if int64(len(buf)) > h.MaxFileSize {
		writeError(w, http.StatusBadRequest, "file_too_large")
		return
	}

	meta := map[string]any{}
	if raw := strings.TrimSpace(r.FormValue("metadata")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_metadata_json")
			return
		}
	}

	mimeType := strings.TrimSpace(r.Header.Get("X-File-Content-Type"))
	if mimeType == "" {
		mimeType = header.Header.Get("Content-Type")
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	doc, err := h.Service.Upload(r.Context(), principal.TenantID, vault.UploadInput{
		CaseID:     caseID,
		Filename:   header.Filename,
		MimeType:   mimeType,
		Data:       buf,
		Metadata:   meta,
		UploadedBy: principal.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *VaultHandlers) List(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, err := uuid.Parse(r.PathValue("case_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_case_id")
		return
	}
	items, err := h.Service.List(r.Context(), principal.TenantID, caseID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *VaultHandlers) Download(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, docID, ok := parseDocPath(w, r)
	if !ok {
		return
	}
	doc, data, err := h.Service.Download(r.Context(), principal.TenantID, caseID, docID, principal.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", doc.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", doc.Filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *VaultHandlers) SignedURL(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, docID, ok := parseDocPath(w, r)
	if !ok {
		return
	}
	expirySeconds, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("expiry")))
	result, err := h.Service.SignedURL(r.Context(), principal.TenantID, caseID, docID, expirySeconds)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *VaultHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, docID, ok := parseDocPath(w, r)
	if !ok {
		return
	}
	if err := h.Service.Delete(r.Context(), principal.TenantID, caseID, docID, principal.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (h *VaultHandlers) Erasure(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req vault.ErasureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Service.Erase(r.Context(), principal.TenantID, req, principal.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "erasure_completed"})
}

func (h *VaultHandlers) SignedDownload(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(r.PathValue("doc_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_doc_id")
		return
	}
	caseID, err := uuid.Parse(r.URL.Query().Get("case_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_case_id")
		return
	}
	uri := strings.TrimSpace(r.URL.Query().Get("uri"))
	exp := strings.TrimSpace(r.URL.Query().Get("exp"))
	sig := strings.TrimSpace(r.URL.Query().Get("sig"))
	doc, data, err := h.Service.DownloadFromSignedURL(r.Context(), docID, caseID, uri, exp, sig)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "expired") || strings.Contains(strings.ToLower(err.Error()), "signature") {
			writeError(w, http.StatusUnauthorized, "invalid_signed_url")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", doc.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", doc.Filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func parseDocPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	caseID, err := uuid.Parse(r.PathValue("case_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_case_id")
		return uuid.Nil, uuid.Nil, false
	}
	docID, err := uuid.Parse(r.PathValue("doc_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_doc_id")
		return uuid.Nil, uuid.Nil, false
	}
	return caseID, docID, true
}

func maxDocumentSizeFromEnv() int64 {
	const defaultBytes = 100 * 1024 * 1024
	raw := strings.TrimSpace(os.Getenv("ACERYX_MAX_DOCUMENT_SIZE"))
	if raw == "" {
		return defaultBytes
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
		return n
	}
	return defaultBytes
}
