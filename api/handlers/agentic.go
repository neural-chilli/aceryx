package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/agentic"
)

type AgenticHandlers struct {
	API *agentic.API
}

func NewAgenticHandlers(api *agentic.API) *AgenticHandlers {
	return &AgenticHandlers{API: api}
}

func (h *AgenticHandlers) ListTraces(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("case_id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_case_id")
		return
	}
	items, err := h.API.ListByCase(r.Context(), principal.TenantID, caseID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AgenticHandlers) GetTrace(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	traceID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	trace, err := h.API.GetTrace(r.Context(), principal.TenantID, traceID)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

func (h *AgenticHandlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	traceID, ok := parseUUIDPath(w, r, "id", "invalid_id")
	if !ok {
		return
	}
	events, err := h.API.GetEvents(r.Context(), principal.TenantID, traceID, r.URL.Query().Get("type"))
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}
