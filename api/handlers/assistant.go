package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/assistant"
)

type AssistantHandlers struct {
	API *assistant.API
}

func NewAssistantHandlers(api *assistant.API) *AssistantHandlers {
	return &AssistantHandlers{API: api}
}

func (h *AssistantHandlers) Stream(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "stream_not_implemented")
}

func (h *AssistantHandlers) Message(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req assistant.MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	resp, err := h.API.Message(r.Context(), principal.TenantID, principal.ID, req)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "required") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AssistantHandlers) CreateSession(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req assistant.CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.PageContext = "builder"
	}
	out, err := h.API.CreateSession(r.Context(), principal.TenantID, principal.ID, req.PageContext)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *AssistantHandlers) GetSession(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	out, err := h.API.GetSession(r.Context(), principal.TenantID, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *AssistantHandlers) DeleteSession(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.API.DeleteSession(r.Context(), principal.TenantID, id); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (h *AssistantHandlers) ApplyDiff(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.API.ApplyDiff(r.Context(), principal.TenantID, id); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "applied"})
}

func (h *AssistantHandlers) RejectDiff(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	if err := h.API.RejectDiff(r.Context(), principal.TenantID, id); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "rejected"})
}

func (h *AssistantHandlers) ListDiffs(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workflowRaw := strings.TrimSpace(r.URL.Query().Get("workflow_id"))
	workflowID, err := uuid.Parse(workflowRaw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workflow_id")
		return
	}
	items, err := h.API.ListDiffs(r.Context(), principal.TenantID, workflowID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
