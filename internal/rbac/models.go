package rbac

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

var (
	DefaultSessionTTL       = 24 * time.Hour
	DefaultPermissionTTL    = 60 * time.Second
	DefaultSessionSweepTick = time.Hour
)

type Principal struct {
	ID        uuid.UUID       `json:"id"`
	TenantID  uuid.UUID       `json:"tenant_id"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Email     string          `json:"email,omitempty"`
	Status    string          `json:"status"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Roles     []string        `json:"roles,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type Role struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
}

type TenantContext struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Branding    json.RawMessage `json:"branding"`
	Terminology json.RawMessage `json:"terminology"`
	Settings    json.RawMessage `json:"settings"`
}

type CreatePrincipalRequest struct {
	Type     string          `json:"type"`
	Name     string          `json:"name"`
	Email    string          `json:"email,omitempty"`
	Password string          `json:"password,omitempty"`
	Roles    []string        `json:"roles"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type UpdatePrincipalRequest struct {
	Name   *string  `json:"name,omitempty"`
	Email  *string  `json:"email,omitempty"`
	Status *string  `json:"status,omitempty"`
	Roles  []string `json:"roles,omitempty"`
}

type CreateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type UpdateRolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

type LoginRequest struct {
	TenantID   *uuid.UUID `json:"tenant_id,omitempty"`
	TenantSlug string     `json:"tenant_slug,omitempty"`
	Email      string     `json:"email"`
	Password   string     `json:"password"`
	IPAddress  string     `json:"-"`
	UserAgent  string     `json:"-"`
}

type LoginResponse struct {
	Token     string        `json:"token"`
	Principal Principal     `json:"principal"`
	Tenant    TenantContext `json:"tenant"`
	ExpiresAt time.Time     `json:"expires_at"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type UpdatePreferencesRequest struct {
	ThemeID       *uuid.UUID      `json:"theme_id,omitempty"`
	Locale        string          `json:"locale,omitempty"`
	Notifications json.RawMessage `json:"notifications,omitempty"`
	Preferences   json.RawMessage `json:"preferences,omitempty"`
}

type UserPreferences struct {
	PrincipalID   uuid.UUID       `json:"principal_id"`
	ThemeID       *uuid.UUID      `json:"theme_id,omitempty"`
	Locale        string          `json:"locale"`
	Notifications json.RawMessage `json:"notifications"`
	Preferences   json.RawMessage `json:"preferences"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type AuthPrincipal struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	SessionID *uuid.UUID
	Type      string
	Name      string
	Email     string
	Roles     []string
}
