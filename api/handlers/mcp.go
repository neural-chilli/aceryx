package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/mcp"
)

type MCPHandlers struct {
	API *mcp.API
}

func NewMCPHandlers(api *mcp.API) *MCPHandlers {
	return &MCPHandlers{API: api}
}

func (h *MCPHandlers) Discover(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req mcp.DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	tools, err := h.API.Discover(r.Context(), principal.TenantID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func (h *MCPHandlers) List(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	servers, err := h.API.List(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

func (h *MCPHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	serverURL := strings.TrimSpace(r.URL.Query().Get("server_url"))
	if serverURL == "" {
		if pathValue := strings.TrimSpace(r.PathValue("url")); pathValue != "" {
			unescaped, err := url.PathUnescape(pathValue)
			if err == nil {
				serverURL = strings.TrimSpace(unescaped)
			}
		}
	}
	if serverURL == "" {
		writeError(w, http.StatusBadRequest, "server_url is required")
		return
	}
	if err := h.API.Delete(r.Context(), principal.TenantID, serverURL); err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (h *MCPHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req mcp.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	tools, err := h.API.Refresh(r.Context(), principal.TenantID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}
