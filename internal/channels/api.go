package channels

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
)

type API struct {
	Store   ChannelStore
	Manager *ChannelManager
}

func NewAPI(store ChannelStore, manager *ChannelManager) *API {
	return &API{Store: store, Manager: manager}
}

func (a *API) List(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := a.Store.List(r.Context(), principal.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "channels_list_failed")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) Create(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req Channel
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if req.ID == uuid.Nil {
		req.ID = uuid.New()
	}
	req.TenantID = principal.TenantID
	if err := a.Store.Create(r.Context(), &req); err != nil {
		writeError(w, http.StatusBadRequest, "channel_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (a *API) Get(w http.ResponseWriter, r *http.Request) {
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
	item, err := a.Store.Get(r.Context(), principal.TenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "channel_not_found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *API) Update(w http.ResponseWriter, r *http.Request) {
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
	var req Channel
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.ID = id
	req.TenantID = principal.TenantID
	if err := a.Store.Update(r.Context(), &req); err != nil {
		writeError(w, http.StatusBadRequest, "channel_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (a *API) Delete(w http.ResponseWriter, r *http.Request) {
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
	if err := a.Store.SoftDelete(r.Context(), principal.TenantID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "channel_delete_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (a *API) Enable(w http.ResponseWriter, r *http.Request) {
	id, ok := a.parseManagedChannelID(w, r)
	if !ok {
		return
	}
	if err := a.Manager.Enable(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "channel_enable_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "enabled"})
}

func (a *API) Disable(w http.ResponseWriter, r *http.Request) {
	id, ok := a.parseManagedChannelID(w, r)
	if !ok {
		return
	}
	if err := a.Manager.Disable(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "channel_disable_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "disabled"})
}

func (a *API) Events(w http.ResponseWriter, r *http.Request) {
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
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := a.Store.ListEvents(r.Context(), principal.TenantID, id, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "channel_events_failed")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) parseManagedChannelID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return uuid.Nil, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code, "code": code})
}
