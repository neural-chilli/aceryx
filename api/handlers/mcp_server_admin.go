package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type MCPServerAdminHandlers struct {
	API *mcpserver.KeyAPI
}

func NewMCPServerAdminHandlers(api *mcpserver.KeyAPI) *MCPServerAdminHandlers {
	return &MCPServerAdminHandlers{API: api}
}

func (h *MCPServerAdminHandlers) ListKeys(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := h.API.List(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": items})
}

func (h *MCPServerAdminHandlers) CreateKey(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req struct {
		UserID  string   `json:"user_id"`
		Name    string   `json:"name"`
		Roles   []string `json:"roles"`
		Enabled *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	uid, err := uuid.Parse(strings.TrimSpace(req.UserID))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	created, err := h.API.Create(r.Context(), principal.TenantID, mcpserver.CreateKeyInput{UserID: uid, Name: req.Name, Roles: req.Roles, Enabled: enabled})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *MCPServerAdminHandlers) UpdateKey(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	var req struct {
		Name    string   `json:"name"`
		Roles   []string `json:"roles"`
		Enabled bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	updated, err := h.API.Update(r.Context(), principal.TenantID, id, mcpserver.UpdateKeyInput{Name: req.Name, Roles: req.Roles, Enabled: req.Enabled})
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *MCPServerAdminHandlers) DeleteKey(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	if err := h.API.Revoke(r.Context(), principal.TenantID, id); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "revoked"})
}

func (h *MCPServerAdminHandlers) GetConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.API.GetConfig())
}

func (h *MCPServerAdminHandlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.API.GetConfig()
	var req struct {
		Enabled           *bool          `json:"enabled"`
		ListenAddr        *string        `json:"listen_addr"`
		AuthType          *string        `json:"auth_type"`
		AuthHeader        *string        `json:"auth_header"`
		EnabledTools      []string       `json:"enabled_tools"`
		DisabledTools     []string       `json:"disabled_tools"`
		RequestsPerMinute *int           `json:"requests_per_minute"`
		ToolLimits        map[string]int `json:"tool_limits"`
		MaxDepth          *int           `json:"max_depth"`
		MaxTimeoutMS      *int           `json:"max_timeout_ms"`
		MaxRequestTimeout *time.Duration `json:"max_request_timeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.ListenAddr != nil {
		cfg.ListenAddr = strings.TrimSpace(*req.ListenAddr)
	}
	if req.AuthType != nil {
		cfg.AuthType = strings.TrimSpace(*req.AuthType)
	}
	if req.AuthHeader != nil {
		cfg.AuthHeader = strings.TrimSpace(*req.AuthHeader)
	}
	if req.RequestsPerMinute != nil {
		cfg.RateLimit.RequestsPerMinute = *req.RequestsPerMinute
	}
	if req.ToolLimits != nil {
		cfg.RateLimit.ToolLimits = req.ToolLimits
	}
	if req.EnabledTools != nil {
		cfg.EnabledTools = req.EnabledTools
	}
	if req.DisabledTools != nil {
		cfg.DisabledTools = req.DisabledTools
	}
	if req.MaxDepth != nil {
		cfg.MaxDepth = *req.MaxDepth
	}
	if req.MaxTimeoutMS != nil {
		cfg.MaxRequestTimeout = time.Duration(*req.MaxTimeoutMS) * time.Millisecond
	}
	if req.MaxRequestTimeout != nil && *req.MaxRequestTimeout > 0 {
		cfg.MaxRequestTimeout = *req.MaxRequestTimeout
	}

	updated, err := h.API.UpdateConfig(r.Context(), cfg)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
