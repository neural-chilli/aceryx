package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/plugins"
)

type PluginHandlers struct {
	Runtime plugins.PluginRuntime
	Store   *plugins.Store
}

func NewPluginHandlers(runtime plugins.PluginRuntime, store *plugins.Store) *PluginHandlers {
	return &PluginHandlers{Runtime: runtime, Store: store}
}

func (h *PluginHandlers) List(w http.ResponseWriter, _ *http.Request) {
	if h.Runtime == nil {
		writeJSON(w, http.StatusOK, []*plugins.Plugin{})
		return
	}
	writeJSON(w, http.StatusOK, h.Runtime.List())
}

func (h *PluginHandlers) Get(w http.ResponseWriter, r *http.Request) {
	if h.Runtime == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found")
		return
	}
	ref, err := parseRefFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_plugin_ref")
		return
	}
	p, err := h.Runtime.Get(ref)
	if err != nil {
		writeError(w, http.StatusNotFound, "plugin_not_found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *PluginHandlers) ListVersions(w http.ResponseWriter, r *http.Request) {
	if h.Runtime == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found")
		return
	}
	pluginID := strings.TrimSpace(r.PathValue("id"))
	if pluginID == "" {
		writeError(w, http.StatusBadRequest, "invalid_plugin_id")
		return
	}
	items, err := h.Runtime.ListVersions(pluginID)
	if err != nil {
		writeError(w, http.StatusNotFound, "plugin_not_found")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *PluginHandlers) Reload(w http.ResponseWriter, r *http.Request) {
	if h.Runtime == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found")
		return
	}
	ref, err := parseRefFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_plugin_ref")
		return
	}
	if err := h.Runtime.Reload(ref); err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *PluginHandlers) Disable(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, plugins.PluginDisabled)
}

func (h *PluginHandlers) Enable(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, plugins.PluginActive)
}

func (h *PluginHandlers) setStatus(w http.ResponseWriter, r *http.Request, status plugins.PluginStatus) {
	if h.Runtime == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found")
		return
	}
	pluginID := strings.TrimSpace(r.PathValue("id"))
	if pluginID == "" {
		writeError(w, http.StatusBadRequest, "invalid_plugin_id")
		return
	}
	items, err := h.Runtime.ListVersions(pluginID)
	if err != nil {
		writeError(w, http.StatusNotFound, "plugin_not_found")
		return
	}
	for _, item := range items {
		item.Status = status
	}
	if rt, ok := h.Runtime.(*plugins.Runtime); ok {
		if err := rt.SetStatus(pluginID, status); err != nil {
			writeInternalServerError(w, r, err)
			return
		}
	}
	if h.Store != nil {
		if err := h.Store.SetStatusByPluginID(r.Context(), pluginID, status); err != nil {
			writeInternalServerError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *PluginHandlers) Invocations(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	pluginID := strings.TrimSpace(r.PathValue("id"))
	if pluginID == "" {
		writeError(w, http.StatusBadRequest, "invalid_plugin_id")
		return
	}
	if h.Store == nil {
		writeJSON(w, http.StatusOK, []plugins.InvocationRecord{})
		return
	}
	items, err := h.Store.ListInvocationsByPlugin(r.Context(), principal.TenantID, pluginID, 100)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *PluginHandlers) StepPalette(w http.ResponseWriter, _ *http.Request) {
	if h.Runtime == nil {
		writeJSON(w, http.StatusOK, []plugins.PaletteCategory{})
		return
	}
	writeJSON(w, http.StatusOK, h.Runtime.StepPalette())
}

func (h *PluginHandlers) ToolPalette(w http.ResponseWriter, _ *http.Request) {
	if h.Runtime == nil {
		writeJSON(w, http.StatusOK, []plugins.PaletteCategory{})
		return
	}
	writeJSON(w, http.StatusOK, h.Runtime.ToolPalette())
}

func (h *PluginHandlers) Search(w http.ResponseWriter, r *http.Request) {
	if h.Runtime == nil {
		writeJSON(w, http.StatusOK, []*plugins.Plugin{})
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	writeJSON(w, http.StatusOK, h.Runtime.Search(query))
}

func (h *PluginHandlers) SchemaChanges(w http.ResponseWriter, r *http.Request) {
	if h.Runtime == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found")
		return
	}
	pluginID := strings.TrimSpace(r.PathValue("id"))
	if pluginID == "" {
		writeError(w, http.StatusBadRequest, "invalid_plugin_id")
		return
	}
	report, ok := h.Runtime.LastSchemaChange(pluginID)
	if !ok {
		writeError(w, http.StatusNotFound, "schema_change_not_found")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func parseRefFromRequest(r *http.Request) (plugins.PluginRef, error) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return plugins.PluginRef{}, fmt.Errorf("plugin id is required")
	}
	if strings.Contains(id, "@") {
		return plugins.ParsePluginRefStrict(id)
	}
	return plugins.PluginRef{ID: id}, nil
}
