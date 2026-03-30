package tenants

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type BrandingColors struct {
	Primary   string `json:"primary"`
	Secondary string `json:"secondary"`
	Accent    string `json:"accent"`
}

type Branding struct {
	CompanyName string         `json:"company_name"`
	LogoURL     string         `json:"logo_url,omitempty"`
	FaviconURL  string         `json:"favicon_url,omitempty"`
	Colors      BrandingColors `json:"colors"`
	PoweredBy   bool           `json:"powered_by"`
}

type Terminology map[string]string

type TenantSettings map[string]any

type Tenant struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	Branding    Branding       `json:"branding"`
	Terminology Terminology    `json:"terminology"`
	Settings    TenantSettings `json:"settings"`
	CreatedAt   time.Time      `json:"created_at"`
}

type Theme struct {
	ID        uuid.UUID       `json:"id"`
	TenantID  uuid.UUID       `json:"tenant_id"`
	Name      string          `json:"name"`
	Key       string          `json:"key"`
	Preset    string          `json:"preset"`
	Mode      string          `json:"mode"`
	Overrides json.RawMessage `json:"overrides"`
	IsDefault bool            `json:"is_default"`
	SortOrder int             `json:"sort_order"`
}

type CreateThemeRequest struct {
	Name      string          `json:"name"`
	Key       string          `json:"key"`
	Preset    string          `json:"preset"`
	Mode      string          `json:"mode"`
	Overrides json.RawMessage `json:"overrides"`
	IsDefault bool            `json:"is_default"`
	SortOrder int             `json:"sort_order"`
}

type UpdateThemeRequest struct {
	Name      *string          `json:"name,omitempty"`
	Key       *string          `json:"key,omitempty"`
	Preset    *string          `json:"preset,omitempty"`
	Mode      *string          `json:"mode,omitempty"`
	Overrides *json.RawMessage `json:"overrides,omitempty"`
	IsDefault *bool            `json:"is_default,omitempty"`
	SortOrder *int             `json:"sort_order,omitempty"`
}

var defaultTerminology = Terminology{
	"case":    "case",
	"cases":   "cases",
	"Case":    "Case",
	"Cases":   "Cases",
	"task":    "task",
	"tasks":   "tasks",
	"Task":    "Task",
	"Tasks":   "Tasks",
	"inbox":   "inbox",
	"Inbox":   "Inbox",
	"reports": "reports",
	"Reports": "Reports",
}

func ResolveTerminology(overrides Terminology) Terminology {
	out := Terminology{}
	for k, v := range defaultTerminology {
		out[k] = v
	}
	for k, v := range overrides {
		if v != "" {
			out[k] = v
		}
	}
	return out
}
