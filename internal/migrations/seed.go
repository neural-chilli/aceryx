package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type roleSeed struct {
	description string
	permissions []string
}

var allPermissions = []string{
	"cases:create",
	"cases:read",
	"cases:update",
	"cases:assign",
	"cases:close",
	"tasks:claim",
	"tasks:complete",
	"tasks:reassign",
	"tasks:escalate",
	"workflows:view",
	"workflows:deploy",
	"workflows:edit",
	"vault:upload",
	"vault:download",
	"vault:delete",
	"admin:users",
	"admin:roles",
	"admin:tenant",
	"admin:audit",
	"reports:query",
}

var defaultRoleSeeds = map[string]roleSeed{
	"admin": {
		description: "Full access to all tenant resources",
		permissions: allPermissions,
	},
	"workflow_designer": {
		description: "Design and deploy workflows",
		permissions: []string{"workflows:*", "cases:read"},
	},
	"case_worker": {
		description: "Work assigned cases and tasks",
		permissions: []string{"cases:read", "cases:update", "tasks:claim", "tasks:complete", "vault:download", "vault:upload", "reports:query"},
	},
	"viewer": {
		description: "Read-only case access",
		permissions: []string{"cases:read", "vault:download"},
	},
}

// SeedDefaultData seeds a default tenant, admin principal, roles, and assignments.
func SeedDefaultData(ctx context.Context, db *sql.DB) error {
	passwordHashBytes, err := bcrypt.GenerateFromPassword([]byte("admin"), 12)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	branding := map[string]any{
		"company_name": "Aceryx Demo",
		"logo_url":     "https://example.com/logo.svg",
		"favicon_url":  "https://example.com/favicon.ico",
		"colors": map[string]any{
			"primary":   "#1f6feb",
			"secondary": "#0f172a",
			"accent":    "#f59e0b",
		},
		"powered_by": true,
	}
	terminology := map[string]any{
		"case":    "case",
		"cases":   "cases",
		"task":    "task",
		"tasks":   "tasks",
		"inbox":   "inbox",
		"reports": "reports",
	}
	settings := map[string]any{
		"default_theme":   "light",
		"sla_warning_pct": 0.75,
	}

	brandingJSON, err := json.Marshal(branding)
	if err != nil {
		return fmt.Errorf("marshal tenant branding: %w", err)
	}
	terminologyJSON, err := json.Marshal(terminology)
	if err != nil {
		return fmt.Errorf("marshal tenant terminology: %w", err)
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal tenant settings: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}

	var tenantID string
	err = tx.QueryRowContext(ctx, `
INSERT INTO tenants (name, slug, branding, terminology, settings)
VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb)
ON CONFLICT (slug) DO UPDATE
SET
    name = EXCLUDED.name,
    branding = EXCLUDED.branding,
    terminology = EXCLUDED.terminology,
    settings = EXCLUDED.settings
RETURNING id
`, "Default Tenant", "default", string(brandingJSON), string(terminologyJSON), string(settingsJSON)).Scan(&tenantID)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("upsert default tenant: %w", err)
	}

	var principalID string
	err = tx.QueryRowContext(ctx, `
INSERT INTO principals (tenant_id, type, name, email, password_hash, status)
VALUES ($1, 'human', $2, $3, $4, 'active')
ON CONFLICT (tenant_id, email) DO UPDATE
SET
    type = EXCLUDED.type,
    name = EXCLUDED.name,
    password_hash = EXCLUDED.password_hash,
    status = EXCLUDED.status
RETURNING id
`, tenantID, "Administrator", "admin@localhost", string(passwordHashBytes)).Scan(&principalID)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("upsert admin principal: %w", err)
	}

	for roleName, role := range defaultRoleSeeds {
		var roleID string
		err = tx.QueryRowContext(ctx, `
INSERT INTO roles (tenant_id, name, description)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, name) DO UPDATE
SET description = EXCLUDED.description
RETURNING id
`, tenantID, roleName, role.description).Scan(&roleID)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert role %s: %w", roleName, err)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("clear role permissions for %s: %w", roleName, err)
		}

		for _, permission := range role.permissions {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				roleID,
				permission,
			); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("assign permission %s to role %s: %w", permission, roleName, err)
			}
		}

		if roleName == "admin" {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO principal_roles (principal_id, role_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`, principalID, roleID); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("assign admin role: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed transaction: %w", err)
	}

	return nil
}
