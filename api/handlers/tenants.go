package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/api/middleware"
	"github.com/neural-chilli/aceryx/internal/tenants"
)

type TenantHandlers struct {
	Tenants *tenants.TenantService
	Themes  *tenants.ThemeService
}

func NewTenantHandlers(ts *tenants.TenantService, th *tenants.ThemeService) *TenantHandlers {
	return &TenantHandlers{Tenants: ts, Themes: th}
}

func (h *TenantHandlers) GetBranding(w http.ResponseWriter, r *http.Request) {
	if slug := strings.TrimSpace(r.URL.Query().Get("slug")); slug != "" {
		branding, err := h.Tenants.GetBrandingBySlug(r.Context(), slug)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "not_found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, branding)
		return
	}

	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	branding, err := h.Tenants.GetBranding(r.Context(), principal.TenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, branding)
}

func (h *TenantHandlers) PutBranding(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	current, err := h.Tenants.GetBranding(r.Context(), principal.TenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		branding, berr := h.updateBrandingFromMultipart(r, principal.TenantID, principal.ID, current)
		if berr != nil {
			writeError(w, http.StatusBadRequest, berr.Error())
			return
		}
		updated, uerr := h.Tenants.UpdateBranding(r.Context(), principal.TenantID, branding)
		if uerr != nil {
			writeError(w, http.StatusInternalServerError, uerr.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}

	payload := tenants.Branding{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if payload.CompanyName == "" {
		payload.CompanyName = current.CompanyName
	}
	if payload.Colors.Primary == "" {
		payload.Colors.Primary = current.Colors.Primary
	}
	if payload.Colors.Secondary == "" {
		payload.Colors.Secondary = current.Colors.Secondary
	}
	if payload.Colors.Accent == "" {
		payload.Colors.Accent = current.Colors.Accent
	}
	if payload.LogoURL == "" {
		payload.LogoURL = current.LogoURL
	}
	if payload.FaviconURL == "" {
		payload.FaviconURL = current.FaviconURL
	}

	updated, err := h.Tenants.UpdateBranding(r.Context(), principal.TenantID, payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *TenantHandlers) updateBrandingFromMultipart(r *http.Request, tenantID, principalID uuid.UUID, current tenants.Branding) (tenants.Branding, error) {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		return tenants.Branding{}, err
	}
	updated := current
	if name := strings.TrimSpace(r.FormValue("company_name")); name != "" {
		updated.CompanyName = name
	}
	if v := strings.TrimSpace(r.FormValue("powered_by")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return tenants.Branding{}, err
		}
		updated.PoweredBy = b
	}
	if primary := strings.TrimSpace(r.FormValue("primary")); primary != "" {
		updated.Colors.Primary = primary
	}
	if secondary := strings.TrimSpace(r.FormValue("secondary")); secondary != "" {
		updated.Colors.Secondary = secondary
	}
	if accent := strings.TrimSpace(r.FormValue("accent")); accent != "" {
		updated.Colors.Accent = accent
	}

	logoURL, err := h.maybeUpload(r, tenantID, principalID, "logo")
	if err != nil {
		return tenants.Branding{}, err
	}
	if logoURL != "" {
		updated.LogoURL = logoURL
	}
	faviconURL, err := h.maybeUpload(r, tenantID, principalID, "favicon")
	if err != nil {
		return tenants.Branding{}, err
	}
	if faviconURL != "" {
		updated.FaviconURL = faviconURL
	}
	return updated, nil
}

func (h *TenantHandlers) maybeUpload(r *http.Request, tenantID, principalID uuid.UUID, field string) (string, error) {
	file, hdr, err := r.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil
		}
		return "", err
	}
	defer func() { _ = file.Close() }()
	buf, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		return "", err
	}
	mimeType := tenants.NormalizeAssetMimeType(hdr.Header.Get("Content-Type"), hdr.Filename)
	return h.Tenants.UploadTenantAsset(r.Context(), tenantID, principalID, hdr.Filename, mimeType, buf)
}

func (h *TenantHandlers) GetTerminology(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	terms, err := h.Tenants.GetTerminology(r.Context(), principal.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, terms)
}

func (h *TenantHandlers) PutTerminology(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	updated, err := h.Tenants.UpdateTerminology(r.Context(), principal.TenantID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *TenantHandlers) ListThemes(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	items, err := h.Themes.ListThemes(r.Context(), principal.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *TenantHandlers) CreateTheme(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req tenants.CreateThemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	created, err := h.Themes.CreateTheme(r.Context(), principal.TenantID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *TenantHandlers) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	themeID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	var req tenants.UpdateThemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	updated, err := h.Themes.UpdateTheme(r.Context(), principal.TenantID, themeID, req)
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

func (h *TenantHandlers) DeleteTheme(w http.ResponseWriter, r *http.Request) {
	principal := middleware.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	themeID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	if err := h.Themes.DeleteTheme(r.Context(), principal.TenantID, themeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
