package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/audit"
)

type AuditHandlers struct {
	Audit *audit.Service
}

func NewAuditHandlers(svc *audit.Service) *AuditHandlers {
	return &AuditHandlers{Audit: svc}
}

func (h *AuditHandlers) ListCaseEvents(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	allowed, err := h.Audit.CaseInTenant(r.Context(), caseID, principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	out, err := h.Audit.ListCaseEvents(r.Context(), caseID, audit.ListFilter{
		EventType: q.Get("event_type"),
		Action:    q.Get("action"),
		Page:      page,
		PerPage:   perPage,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *AuditHandlers) VerifyCaseEvents(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	allowed, err := h.Audit.CaseInTenant(r.Context(), caseID, principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	result, err := h.Audit.VerifyCaseChain(r.Context(), caseID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AuditHandlers) ExportCaseEvents(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	caseID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	allowed, err := h.Audit.CaseInTenant(r.Context(), caseID, principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	format := r.URL.Query().Get("format")
	switch format {
	case "", "json":
		out, err := h.Audit.ExportCaseEventsJSON(r.Context(), caseID)
		if err != nil {
			writeInternalServerError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	case "csv":
		out, err := h.Audit.ExportCaseEventsCSV(r.Context(), caseID)
		if err != nil {
			writeInternalServerError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	default:
		writeError(w, http.StatusBadRequest, "invalid_format")
	}
}

func (h *AuditHandlers) DecodeEventData(raw []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}
