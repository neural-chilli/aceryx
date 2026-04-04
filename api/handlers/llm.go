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
	"github.com/neural-chilli/aceryx/internal/llm"
)

type LLMAdminHandlers struct {
	Store   *llm.Store
	Manager *llm.AdapterManager
}

func NewLLMAdminHandlers(store *llm.Store, manager *llm.AdapterManager) *LLMAdminHandlers {
	return &LLMAdminHandlers{Store: store, Manager: manager}
}

func (h *LLMAdminHandlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := h.Store.ListProviders(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	for i := range items {
		items[i].APIKeySecret = ""
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *LLMAdminHandlers) CreateProvider(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req llm.LLMProviderConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.TenantID = principal.TenantID
	if strings.TrimSpace(req.Provider) == "" || strings.TrimSpace(req.DefaultModel) == "" {
		writeError(w, http.StatusBadRequest, "provider and default_model are required")
		return
	}
	created, err := h.Store.CreateProvider(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.Manager != nil {
		if err := h.Manager.AddProvider(created); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	created.APIKeySecret = ""
	writeJSON(w, http.StatusCreated, created)
}

func (h *LLMAdminHandlers) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	current, err := h.Store.GetProvider(r.Context(), principal.TenantID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	var req llm.LLMProviderConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.ID = id
	req.TenantID = principal.TenantID
	if req.APIKeySecret == "" {
		req.APIKeySecret = current.APIKeySecret
	}
	updated, err := h.Store.UpdateProvider(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.Manager != nil {
		_ = h.Manager.RemoveProvider(id)
		if err := h.Manager.AddProvider(updated); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	updated.APIKeySecret = ""
	writeJSON(w, http.StatusOK, updated)
}

func (h *LLMAdminHandlers) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	if err := h.Store.DeleteProvider(r.Context(), principal.TenantID, id); err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	if h.Manager != nil {
		_ = h.Manager.RemoveProvider(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *LLMAdminHandlers) TestProvider(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	cfg, err := h.Store.GetProvider(r.Context(), principal.TenantID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	if h.Manager == nil {
		writeError(w, http.StatusServiceUnavailable, "llm_manager_unavailable")
		return
	}
	if err := h.Manager.TestProvider(r.Context(), cfg.ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *LLMAdminHandlers) UsageSummary(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	yearMonth := strings.TrimSpace(r.URL.Query().Get("year_month"))
	if yearMonth == "" {
		yearMonth = time.Now().UTC().Format("2006-01")
	}
	usage, err := h.Store.GetMonthlyUsage(r.Context(), principal.TenantID, yearMonth)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (h *LLMAdminHandlers) UsageDetails(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	since := time.Now().UTC().AddDate(0, -1, 0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_since")
			return
		}
		since = t
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	items, err := h.Store.ListInvocations(r.Context(), principal.TenantID, llm.ListOpts{
		Since:  since,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *LLMAdminHandlers) UsageByPurpose(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	since := time.Now().UTC().AddDate(0, -1, 0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_since")
			return
		}
		since = t
	}
	items, err := h.Store.UsageByPurpose(r.Context(), principal.TenantID, since)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
