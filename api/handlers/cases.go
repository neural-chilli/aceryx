package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/cases"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type CaseHandlers struct {
	CaseTypes *cases.CaseTypeService
	Cases     *cases.CaseService
	Reports   *cases.ReportsService
}

func NewCaseHandlers(ct *cases.CaseTypeService, cs *cases.CaseService, rs *cases.ReportsService) *CaseHandlers {
	return &CaseHandlers{CaseTypes: ct, Cases: cs, Reports: rs}
}

func (h *CaseHandlers) RegisterCaseType(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req struct {
		Name   string               `json:"name"`
		Schema cases.CaseTypeSchema `json:"schema"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	ct, validation, err := h.CaseTypes.RegisterCaseType(r.Context(), principal.TenantID, principal.ID, req.Name, req.Schema)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(validation) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "validation_failed", "details": validation})
		return
	}
	writeJSON(w, http.StatusCreated, ct)
}

func (h *CaseHandlers) ListCaseTypes(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	out, err := h.CaseTypes.ListCaseTypes(r.Context(), principal.TenantID, includeArchived)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *CaseHandlers) GetCaseType(w http.ResponseWriter, r *http.Request) {
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
	ct, err := h.CaseTypes.GetCaseTypeByID(r.Context(), principal.TenantID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ct)
}

func (h *CaseHandlers) CreateCase(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req cases.CreateCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	c, validation, err := h.Cases.CreateCase(r.Context(), principal.TenantID, principal.ID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(validation) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "validation_failed", "details": validation})
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *CaseHandlers) GetCase(w http.ResponseWriter, r *http.Request) {
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
	c, err := h.Cases.GetCase(r.Context(), principal.TenantID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *CaseHandlers) ListCases(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	q := r.URL.Query()
	filter := cases.ListCasesFilter{
		Statuses: splitCSV(q.Get("status")),
		CaseType: q.Get("case_type"),
		SortBy:   q.Get("sort_by"),
		SortDir:  q.Get("sort_dir"),
	}
	if p, _ := strconv.Atoi(q.Get("page")); p > 0 {
		filter.Page = p
	}
	if pp, _ := strconv.Atoi(q.Get("per_page")); pp > 0 {
		filter.PerPage = pp
	}
	if q.Get("assigned_to") == "unassigned" {
		filter.AssignedNone = true
	} else if q.Get("assigned_to") == "me" {
		id := principal.ID
		filter.AssignedTo = &id
	} else if q.Get("assigned_to") != "" {
		if id, err := uuid.Parse(q.Get("assigned_to")); err == nil {
			filter.AssignedTo = &id
		}
	}
	if v := q.Get("priority"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Priority = &n
		}
	}

	out, err := h.Cases.ListCases(r.Context(), principal.TenantID, filter)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *CaseHandlers) PatchCaseData(w http.ResponseWriter, r *http.Request) {
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
	match := r.Header.Get("If-Match")
	if match == "" {
		writeError(w, http.StatusBadRequest, "if_match_required")
		return
	}
	expectedVersion, err := strconv.Atoi(strings.TrimSpace(match))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_if_match")
		return
	}
	patch := map[string]interface{}{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	result, validation, err := h.Cases.UpdateCaseData(r.Context(), principal.TenantID, id, principal.ID, patch, expectedVersion)
	if err != nil {
		if errors.Is(err, engine.ErrCaseDataConflict) {
			writeError(w, http.StatusConflict, "version_mismatch")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	if len(validation) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "validation_failed", "details": validation})
		return
	}
	writeJSON(w, http.StatusOK, result.Case)
}

func (h *CaseHandlers) CloseCase(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.Cases.CloseCase(r.Context(), principal.TenantID, id, principal.ID, req.Reason); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

func (h *CaseHandlers) CancelCase(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.Cases.CancelCase(r.Context(), principal.TenantID, id, principal.ID, req.Reason); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *CaseHandlers) SearchCases(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	q := r.URL.Query()
	filter := cases.SearchFilter{Query: q.Get("q"), CaseType: q.Get("case_type"), Status: q.Get("status")}
	if p, _ := strconv.Atoi(q.Get("page")); p > 0 {
		filter.Page = p
	}
	if pp, _ := strconv.Atoi(q.Get("per_page")); pp > 0 {
		filter.PerPage = pp
	}
	out, err := h.Cases.SearchCases(r.Context(), principal.TenantID, nil, filter)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *CaseHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	q := r.URL.Query()
	filter := cases.DashboardFilter{Statuses: splitCSV(q.Get("status")), CaseType: q.Get("case_type"), SLAStatus: q.Get("sla_status"), SortBy: q.Get("sort_by"), SortDir: q.Get("sort_dir")}
	if p, _ := strconv.Atoi(q.Get("page")); p > 0 {
		filter.Page = p
	}
	if pp, _ := strconv.Atoi(q.Get("per_page")); pp > 0 {
		filter.PerPage = pp
	}
	if d, _ := strconv.Atoi(q.Get("older_than_days")); d > 0 {
		filter.OlderThanDays = &d
	}
	if v := q.Get("priority"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Priority = &n
		}
	}
	if q.Get("assigned_to") == "unassigned" {
		filter.AssignedNone = true
	} else if q.Get("assigned_to") == "me" {
		id := principal.ID
		filter.AssignedTo = &id
	} else if q.Get("assigned_to") != "" {
		if id, err := uuid.Parse(q.Get("assigned_to")); err == nil {
			filter.AssignedTo = &id
		}
	}
	if v := q.Get("created_after"); v != "" {
		if tt, err := time.Parse(time.RFC3339, v); err == nil {
			filter.CreatedAfter = &tt
		}
	}
	if v := q.Get("created_before"); v != "" {
		if tt, err := time.Parse(time.RFC3339, v); err == nil {
			filter.CreatedBefore = &tt
		}
	}

	out, err := h.Cases.Dashboard(r.Context(), principal.TenantID, filter)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *CaseHandlers) ReportCasesSummary(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	weeks, _ := strconv.Atoi(r.URL.Query().Get("weeks"))
	out, err := h.Reports.CasesSummary(r.Context(), principal.TenantID, weeks)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"periods": out})
}

func (h *CaseHandlers) ReportAgeing(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	thresholds := parseThresholds(r.URL.Query().Get("thresholds"))
	out, err := h.Reports.Ageing(r.Context(), principal.TenantID, thresholds)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"brackets": out})
}

func (h *CaseHandlers) ReportSLACompliance(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	weeks, _ := strconv.Atoi(r.URL.Query().Get("weeks"))
	out, err := h.Reports.SLACompliance(r.Context(), principal.TenantID, weeks)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"periods": out})
}

func (h *CaseHandlers) ReportCasesByStage(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	out, err := h.Reports.CasesByStage(r.Context(), principal.TenantID, r.URL.Query().Get("case_type"))
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"stages": out})
}

func (h *CaseHandlers) ReportWorkload(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	out, err := h.Reports.Workload(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": out})
}

func (h *CaseHandlers) ReportDecisions(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	weeks, _ := strconv.Atoi(r.URL.Query().Get("weeks"))
	out, err := h.Reports.Decisions(r.Context(), principal.TenantID, weeks)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"periods": out})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func splitCSV(in string) []string {
	if in == "" {
		return nil
	}
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseThresholds(raw string) []int {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}
