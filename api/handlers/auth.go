package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/observability"
	"github.com/neural-chilli/aceryx/internal/rbac"
)

type AuthHandlers struct {
	Auth       *rbac.AuthService
	Principals *rbac.PrincipalService
	Roles      *rbac.RoleService
}

func NewAuthHandlers(auth *rbac.AuthService, principals *rbac.PrincipalService, roles *rbac.RoleService) *AuthHandlers {
	return &AuthHandlers{Auth: auth, Principals: principals, Roles: roles}
}

func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req rbac.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	if req.TenantID == nil {
		if hdr := strings.TrimSpace(r.Header.Get("X-Tenant-ID")); hdr != "" {
			id, err := uuid.Parse(hdr)
			if err == nil {
				req.TenantID = &id
			}
		}
	}
	if req.TenantSlug == "" {
		req.TenantSlug = strings.TrimSpace(r.Header.Get("X-Tenant-Slug"))
	}
	req.IPAddress = strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if req.IPAddress == "" {
		req.IPAddress = r.RemoteAddr
	}
	req.UserAgent = r.UserAgent()

	resp, err := h.Auth.Login(r.Context(), req)
	if err != nil {
		slog.WarnContext(r.Context(), "login failed",
			append(observability.RequestAttrs(r.Context()),
				"email", req.Email,
			)...,
		)
		if errors.Is(err, rbac.ErrInvalidCredential) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeInternalServerError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "login successful",
		append(observability.RequestAttrs(r.Context()),
			"principal_id", resp.Principal.ID.String(),
			"tenant_id", resp.Principal.TenantID.String(),
		)...,
	)
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil || principal.SessionID == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if err := h.Auth.Logout(r.Context(), principal.TenantID, principal.ID, *principal.SessionID); err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "logout successful",
		append(observability.RequestAttrs(r.Context()),
			"principal_id", principal.ID.String(),
			"tenant_id", principal.TenantID.String(),
		)...,
	)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *AuthHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil || principal.SessionID == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req rbac.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Auth.ChangePassword(r.Context(), principal.TenantID, principal.ID, *principal.SessionID, req); err != nil {
		if errors.Is(err, rbac.ErrInvalidCredential) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *AuthHandlers) GetPreferences(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	pref, err := h.Auth.GetPreferences(r.Context(), principal.TenantID, principal.ID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pref)
}

func (h *AuthHandlers) PutPreferences(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req rbac.UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	pref, err := h.Auth.UpdatePreferences(r.Context(), principal.TenantID, principal.ID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pref)
}

func (h *AuthHandlers) CreatePrincipal(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req rbac.CreatePrincipalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	created, apiKey, err := h.Principals.CreatePrincipal(r.Context(), principal.TenantID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload := map[string]any{"principal": created}
	if apiKey != "" {
		payload["api_key"] = apiKey
	}
	writeJSON(w, http.StatusCreated, payload)
}

func (h *AuthHandlers) ListPrincipals(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	out, err := h.Principals.ListPrincipals(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *AuthHandlers) UpdatePrincipal(w http.ResponseWriter, r *http.Request) {
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
	var req rbac.UpdatePrincipalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	updated, err := h.Principals.UpdatePrincipal(r.Context(), principal.TenantID, id, req)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *AuthHandlers) DisablePrincipal(w http.ResponseWriter, r *http.Request) {
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
	if err := h.Principals.DisablePrincipal(r.Context(), principal.TenantID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "disabled"})
}

func (h *AuthHandlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req rbac.CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	created, err := h.Roles.CreateRole(r.Context(), principal.TenantID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *AuthHandlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	roles, err := h.Roles.ListRoles(r.Context(), principal.TenantID)
	if err != nil {
		writeInternalServerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (h *AuthHandlers) UpdateRolePermissions(w http.ResponseWriter, r *http.Request) {
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
	var req rbac.UpdateRolePermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	role, err := h.Roles.UpdateRolePermissions(r.Context(), principal.TenantID, id, req.Permissions)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, role)
}
