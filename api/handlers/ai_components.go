package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/ai"
)

type AIComponentHandlers struct {
	Registry *ai.ComponentRegistry
}

func NewAIComponentHandlers(registry *ai.ComponentRegistry) *AIComponentHandlers {
	return &AIComponentHandlers{Registry: registry}
}

func (h *AIComponentHandlers) List(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := h.Registry.List(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ai.ListResponse{Items: items})
}

func (h *AIComponentHandlers) Get(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	item, err := h.Registry.Get(r.Context(), principal.TenantID, id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *AIComponentHandlers) Create(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	def, ok := decodeDefinition(w, r)
	if !ok {
		return
	}
	if err := h.Registry.AddTenantComponent(r.Context(), principal.TenantID, principal.ID, def); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, def)
}

func (h *AIComponentHandlers) Update(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	def, ok := decodeDefinition(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(def.ID) == "" {
		def.ID = pathID
	}
	if !strings.EqualFold(def.ID, pathID) {
		writeError(w, http.StatusBadRequest, "id_mismatch")
		return
	}
	if err := h.Registry.UpdateTenantComponent(r.Context(), principal.TenantID, def); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (h *AIComponentHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	if err := h.Registry.DeleteTenantComponent(r.Context(), principal.TenantID, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *AIComponentHandlers) Reload(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if err := h.Registry.Reload(); err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func decodeDefinition(w http.ResponseWriter, r *http.Request) (*ai.AIComponentDef, bool) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return nil, false
	}
	var wrapped ai.UpsertRequest
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&wrapped); err == nil && wrapped.Definition != nil {
		return wrapped.Definition, true
	}
	var def ai.AIComponentDef
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return nil, false
	}
	return &def, true
}
