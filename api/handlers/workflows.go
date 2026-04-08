package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/workflows"
)

type WorkflowHandlers struct {
	Service *workflows.Service
}

func NewWorkflowHandlers(service *workflows.Service) *WorkflowHandlers {
	return &WorkflowHandlers{Service: service}
}

func (h *WorkflowHandlers) List(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := h.Service.List(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *WorkflowHandlers) Create(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req workflows.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	out, err := h.Service.Create(r.Context(), principal.TenantID, principal.ID, req)
	if err != nil {
		if err.Error() == "name is required" || err.Error() == "case_type_id is required" {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *WorkflowHandlers) GetDraft(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workflowID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	ast, err := h.Service.GetDraftAST(r.Context(), principal.TenantID, workflowID)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(ast)
}

func (h *WorkflowHandlers) PutDraft(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workflowID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Service.SaveDraftAST(r.Context(), principal.TenantID, workflowID, raw); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if err.Error() == "ast is required" || strings.HasPrefix(err.Error(), "invalid ast json:") || strings.HasPrefix(err.Error(), "invalid workflow ast:") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved"})
}

func (h *WorkflowHandlers) Publish(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workflowID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.Service.PublishDraft(r.Context(), principal.TenantID, workflowID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if strings.HasPrefix(err.Error(), "invalid workflow ast:") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "published"})
}

func (h *WorkflowHandlers) ExportYAMLLatest(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workflowID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	out, err := h.Service.ExportYAMLLatest(r.Context(), principal.TenantID, workflowID)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out))
}

func (h *WorkflowHandlers) ExportYAMLVersion(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workflowID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	versionRaw := strings.TrimSpace(r.PathValue("version"))
	version, err := strconv.Atoi(versionRaw)
	if err != nil || version <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_version")
		return
	}
	out, err := h.Service.ExportYAMLVersion(r.Context(), principal.TenantID, workflowID, version)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out))
}

func (h *WorkflowHandlers) ImportYAMLDraft(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workflowID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_file")
		return
	}

	if err := h.Service.ImportYAMLDraft(r.Context(), principal.TenantID, workflowID, string(data)); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if err.Error() == "yaml is required" || strings.HasPrefix(err.Error(), "invalid yaml:") || strings.HasPrefix(err.Error(), "invalid workflow ast:") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "imported"})
}
