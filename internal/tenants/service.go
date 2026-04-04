package tenants

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type TenantService struct {
	db *sql.DB
}

type ThemeService struct {
	db *sql.DB
}

func NewTenantService(db *sql.DB) *TenantService {
	return &TenantService{db: db}
}

func NewThemeService(db *sql.DB) *ThemeService {
	return &ThemeService{db: db}
}

func (s *TenantService) GetTenant(ctx context.Context, tenantID uuid.UUID) (Tenant, error) {
	var t Tenant
	var brandingRaw, terminologyRaw, settingsRaw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, slug, branding, terminology, settings, created_at
FROM tenants
WHERE id = $1
`, tenantID).Scan(&t.ID, &t.Name, &t.Slug, &brandingRaw, &terminologyRaw, &settingsRaw, &t.CreatedAt)
	if err != nil {
		return Tenant{}, err
	}
	if err := json.Unmarshal(brandingRaw, &t.Branding); err != nil {
		return Tenant{}, fmt.Errorf("decode tenant branding: %w", err)
	}
	if err := json.Unmarshal(terminologyRaw, &t.Terminology); err != nil {
		return Tenant{}, fmt.Errorf("decode tenant terminology: %w", err)
	}
	if err := json.Unmarshal(settingsRaw, &t.Settings); err != nil {
		return Tenant{}, fmt.Errorf("decode tenant settings: %w", err)
	}
	t.Terminology = ResolveTerminology(t.Terminology)
	return t, nil
}

func (s *TenantService) GetBrandingBySlug(ctx context.Context, slug string) (Branding, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT branding FROM tenants WHERE slug = $1`, slug).Scan(&raw)
	if err != nil {
		return Branding{}, err
	}
	var branding Branding
	if err := json.Unmarshal(raw, &branding); err != nil {
		return Branding{}, fmt.Errorf("decode branding: %w", err)
	}
	return branding, nil
}

func (s *TenantService) UpdateBranding(ctx context.Context, tenantID uuid.UUID, branding Branding) (Branding, error) {
	raw, err := json.Marshal(branding)
	if err != nil {
		return Branding{}, fmt.Errorf("marshal branding: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE tenants SET branding = $2::jsonb WHERE id = $1`, tenantID, string(raw)); err != nil {
		return Branding{}, fmt.Errorf("update branding: %w", err)
	}
	return s.GetBranding(ctx, tenantID)
}

func (s *TenantService) GetBranding(ctx context.Context, tenantID uuid.UUID) (Branding, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT branding FROM tenants WHERE id = $1`, tenantID).Scan(&raw)
	if err != nil {
		return Branding{}, err
	}
	var branding Branding
	if err := json.Unmarshal(raw, &branding); err != nil {
		return Branding{}, fmt.Errorf("decode branding: %w", err)
	}
	return branding, nil
}

func (s *TenantService) UpdateTerminology(ctx context.Context, tenantID uuid.UUID, terms Terminology) (Terminology, error) {
	for _, pair := range [][2]string{{"case", "Case"}, {"cases", "Cases"}, {"task", "Task"}, {"tasks", "Tasks"}, {"inbox", "Inbox"}} {
		_, lowerSet := terms[pair[0]]
		_, upperSet := terms[pair[1]]
		if lowerSet != upperSet {
			return nil, fmt.Errorf("terminology must provide both %s and %s together", pair[0], pair[1])
		}
	}
	merged := ResolveTerminology(terms)
	for _, pair := range [][2]string{{"case", "Case"}, {"cases", "Cases"}, {"task", "Task"}, {"tasks", "Tasks"}, {"inbox", "Inbox"}} {
		if merged[pair[0]] == "" || merged[pair[1]] == "" {
			return nil, fmt.Errorf("terminology must include both %s and %s", pair[0], pair[1])
		}
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal terminology: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE tenants SET terminology = $2::jsonb WHERE id = $1`, tenantID, string(raw)); err != nil {
		return nil, fmt.Errorf("update terminology: %w", err)
	}
	return s.GetTerminology(ctx, tenantID)
}

func (s *TenantService) GetTerminology(ctx context.Context, tenantID uuid.UUID) (Terminology, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT terminology FROM tenants WHERE id = $1`, tenantID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	terms := Terminology{}
	if err := json.Unmarshal(raw, &terms); err != nil {
		return nil, fmt.Errorf("decode terminology: %w", err)
	}
	return ResolveTerminology(terms), nil
}

func (s *TenantService) UpdateSettings(ctx context.Context, tenantID uuid.UUID, settings TenantSettings) (TenantSettings, error) {
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal settings: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE tenants SET settings = $2::jsonb WHERE id = $1`, tenantID, string(raw)); err != nil {
		return nil, fmt.Errorf("update tenant settings: %w", err)
	}
	return s.GetSettings(ctx, tenantID)
}

func (s *TenantService) GetSettings(ctx context.Context, tenantID uuid.UUID) (TenantSettings, error) {
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT settings FROM tenants WHERE id = $1`, tenantID).Scan(&raw); err != nil {
		return nil, err
	}
	settings := TenantSettings{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("decode tenant settings: %w", err)
	}
	return settings, nil
}

func (s *TenantService) UploadTenantAsset(ctx context.Context, tenantID, uploadedBy uuid.UUID, filename, mimeType string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("asset data is empty")
	}
	hash := sha256.Sum256(data)
	h := hex.EncodeToString(hash[:])
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".bin"
	}
	storageURI := fmt.Sprintf("/vault/tenant-assets/%s/%s%s", tenantID.String(), h[:20], ext)
	url := storageURI

	if _, err := s.db.ExecContext(ctx, `
INSERT INTO vault_documents (
    tenant_id, case_id, step_id, filename, mime_type, size_bytes, content_hash, storage_uri, uploaded_by, uploaded_at
)
VALUES ($1, NULL, NULL, $2, $3, $4, $5, $6, $7, now())
`, tenantID, filename, mimeType, len(data), h, storageURI, uploadedBy); err != nil {
		return "", fmt.Errorf("insert tenant asset document: %w", err)
	}
	return url, nil
}

func (s *ThemeService) ListThemes(ctx context.Context, tenantID uuid.UUID) ([]Theme, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, name, key, preset, mode, overrides, is_default, sort_order
FROM themes
WHERE tenant_id = $1
ORDER BY sort_order, name
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list themes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	themes := make([]Theme, 0)
	for rows.Next() {
		var th Theme
		if err := rows.Scan(&th.ID, &th.TenantID, &th.Name, &th.Key, &th.Preset, &th.Mode, &th.Overrides, &th.IsDefault, &th.SortOrder); err != nil {
			return nil, fmt.Errorf("scan theme: %w", err)
		}
		themes = append(themes, th)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate themes: %w", err)
	}
	return themes, nil
}

func (s *ThemeService) CreateTheme(ctx context.Context, tenantID uuid.UUID, req CreateThemeRequest) (Theme, error) {
	if req.Key == "" || req.Name == "" {
		return Theme{}, fmt.Errorf("theme name and key are required")
	}
	if req.Preset == "" {
		req.Preset = "aura"
	}
	if req.Mode == "" {
		req.Mode = "light"
	}
	if len(req.Overrides) == 0 {
		req.Overrides = json.RawMessage("{}")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Theme{}, fmt.Errorf("begin create theme tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if req.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE themes SET is_default = false WHERE tenant_id = $1`, tenantID); err != nil {
			return Theme{}, fmt.Errorf("clear existing default themes: %w", err)
		}
	}

	var th Theme
	err = tx.QueryRowContext(ctx, `
INSERT INTO themes (tenant_id, name, key, preset, mode, overrides, is_default, sort_order)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
RETURNING id, tenant_id, name, key, preset, mode, overrides, is_default, sort_order
`, tenantID, req.Name, req.Key, req.Preset, req.Mode, string(req.Overrides), req.IsDefault, req.SortOrder).Scan(&th.ID, &th.TenantID, &th.Name, &th.Key, &th.Preset, &th.Mode, &th.Overrides, &th.IsDefault, &th.SortOrder)
	if err != nil {
		return Theme{}, fmt.Errorf("insert theme: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Theme{}, fmt.Errorf("commit create theme tx: %w", err)
	}
	return th, nil
}

func (s *ThemeService) UpdateTheme(ctx context.Context, tenantID, themeID uuid.UUID, req UpdateThemeRequest) (Theme, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Theme{}, fmt.Errorf("begin update theme tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := s.getThemeTx(ctx, tx, tenantID, themeID)
	if err != nil {
		return Theme{}, err
	}

	if req.IsDefault != nil && *req.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE themes SET is_default = false WHERE tenant_id = $1`, tenantID); err != nil {
			return Theme{}, fmt.Errorf("clear existing default themes: %w", err)
		}
	}

	name := current.Name
	if req.Name != nil {
		name = *req.Name
	}
	key := current.Key
	if req.Key != nil {
		key = *req.Key
	}
	preset := current.Preset
	if req.Preset != nil {
		preset = *req.Preset
	}
	mode := current.Mode
	if req.Mode != nil {
		mode = *req.Mode
	}
	overrides := current.Overrides
	if req.Overrides != nil {
		overrides = *req.Overrides
	}
	isDefault := current.IsDefault
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}
	sortOrder := current.SortOrder
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	var out Theme
	err = tx.QueryRowContext(ctx, `
UPDATE themes
SET name = $3,
    key = $4,
    preset = $5,
    mode = $6,
    overrides = $7::jsonb,
    is_default = $8,
    sort_order = $9
WHERE tenant_id = $1 AND id = $2
RETURNING id, tenant_id, name, key, preset, mode, overrides, is_default, sort_order
`, tenantID, themeID, name, key, preset, mode, string(overrides), isDefault, sortOrder).Scan(&out.ID, &out.TenantID, &out.Name, &out.Key, &out.Preset, &out.Mode, &out.Overrides, &out.IsDefault, &out.SortOrder)
	if err != nil {
		return Theme{}, fmt.Errorf("update theme: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Theme{}, fmt.Errorf("commit update theme tx: %w", err)
	}
	return out, nil
}

func (s *ThemeService) DeleteTheme(ctx context.Context, tenantID, themeID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete theme tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var wasDefault bool
	err = tx.QueryRowContext(ctx, `SELECT is_default FROM themes WHERE tenant_id = $1 AND id = $2`, tenantID, themeID).Scan(&wasDefault)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("load theme before delete: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE user_preferences SET theme_id = NULL WHERE theme_id = $1`, themeID); err != nil {
		return fmt.Errorf("clear deleted theme references in user preferences: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM themes WHERE tenant_id = $1 AND id = $2`, tenantID, themeID); err != nil {
		return fmt.Errorf("delete theme: %w", err)
	}
	if wasDefault {
		if _, err := tx.ExecContext(ctx, `
UPDATE themes
SET is_default = true
WHERE id = (
    SELECT id
    FROM themes
    WHERE tenant_id = $1
    ORDER BY sort_order, created_at, name
    LIMIT 1
)
`, tenantID); err != nil {
			return fmt.Errorf("reassign default theme after deletion: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete theme tx: %w", err)
	}
	return nil
}

func (s *ThemeService) getThemeTx(ctx context.Context, tx *sql.Tx, tenantID, themeID uuid.UUID) (Theme, error) {
	var th Theme
	err := tx.QueryRowContext(ctx, `
SELECT id, tenant_id, name, key, preset, mode, overrides, is_default, sort_order
FROM themes
WHERE tenant_id = $1 AND id = $2
`, tenantID, themeID).Scan(&th.ID, &th.TenantID, &th.Name, &th.Key, &th.Preset, &th.Mode, &th.Overrides, &th.IsDefault, &th.SortOrder)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Theme{}, sql.ErrNoRows
		}
		return Theme{}, fmt.Errorf("load theme: %w", err)
	}
	return th, nil
}

func (s *ThemeService) GetDefaultTheme(ctx context.Context, tenantID uuid.UUID) (Theme, error) {
	var th Theme
	err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, key, preset, mode, overrides, is_default, sort_order
FROM themes
WHERE tenant_id = $1
ORDER BY is_default DESC, sort_order, name
LIMIT 1
`, tenantID).Scan(&th.ID, &th.TenantID, &th.Name, &th.Key, &th.Preset, &th.Mode, &th.Overrides, &th.IsDefault, &th.SortOrder)
	if err != nil {
		return Theme{}, err
	}
	return th, nil
}

func (s *ThemeService) ResolveThemeForUser(ctx context.Context, tenantID, principalID uuid.UUID) (Theme, error) {
	var preferredID *uuid.UUID
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(up.theme_id::text, '')
FROM principals p
LEFT JOIN user_preferences up ON up.principal_id = p.id
WHERE p.id = $1 AND p.tenant_id = $2
`, principalID, tenantID).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Theme{}, fmt.Errorf("load user preferred theme id: %w", err)
	}
	if raw.Valid && raw.String != "" {
		id, parseErr := uuid.Parse(raw.String)
		if parseErr == nil {
			preferredID = &id
		}
	}
	if preferredID != nil {
		var th Theme
		err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, key, preset, mode, overrides, is_default, sort_order
FROM themes
WHERE tenant_id = $1 AND id = $2
`, tenantID, *preferredID).Scan(&th.ID, &th.TenantID, &th.Name, &th.Key, &th.Preset, &th.Mode, &th.Overrides, &th.IsDefault, &th.SortOrder)
		if err == nil {
			return th, nil
		}
	}
	return s.GetDefaultTheme(ctx, tenantID)
}

func NormalizeAssetMimeType(mime string, fallbackName string) string {
	if mime == "" {
		ext := strings.ToLower(filepath.Ext(fallbackName))
		switch ext {
		case ".png":
			return "image/png"
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".svg":
			return "image/svg+xml"
		case ".ico":
			return "image/x-icon"
		default:
			return "application/octet-stream"
		}
	}
	return mime
}
