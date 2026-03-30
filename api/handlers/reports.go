package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/reports"
)

type ReportsHandlers struct {
	Reports *reports.Service
}

func NewReportsHandlers(svc *reports.Service) *ReportsHandlers {
	return &ReportsHandlers{Reports: svc}
}

func (h *ReportsHandlers) Ask(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	resp, err := h.Reports.Ask(r.Context(), principal.TenantID, req.Question)
	if err != nil {
		writeError(w, http.StatusBadRequest, reports.FriendlyError(err))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *ReportsHandlers) Create(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req reports.SaveReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	out, err := h.Reports.SaveReport(r.Context(), principal.TenantID, principal.ID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, reports.FriendlyError(err))
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *ReportsHandlers) List(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	isAdmin := hasRole(principal, "admin")
	out, err := h.Reports.ListReports(r.Context(), principal.TenantID, principal.ID, scope, isAdmin)
	if err != nil {
		if err.Error() == "forbidden" {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ReportsHandlers) Get(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	out, err := h.Reports.GetReport(r.Context(), principal.TenantID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ReportsHandlers) Run(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	out, err := h.Reports.RunReport(r.Context(), principal.TenantID, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, reports.FriendlyError(err))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ReportsHandlers) Update(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	var req reports.UpdateReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	out, err := h.Reports.UpdateReport(r.Context(), principal.TenantID, principal.ID, id, req, hasRole(principal, "admin"))
	if err != nil {
		if err.Error() == "forbidden" {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ReportsHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	if err := h.Reports.DeleteReport(r.Context(), principal.TenantID, principal.ID, id, hasRole(principal, "admin")); err != nil {
		if err.Error() == "forbidden" {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func hasRole(principal *middleware.Principal, role string) bool {
	if principal == nil {
		return false
	}
	for _, r := range principal.Roles {
		if strings.EqualFold(strings.TrimSpace(r), role) {
			return true
		}
	}
	return false
}
