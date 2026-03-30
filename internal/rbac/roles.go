package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type RoleService struct {
	db    *sql.DB
	authz *Service
}

func NewRoleService(db *sql.DB, authz *Service) *RoleService {
	return &RoleService{db: db, authz: authz}
}

func (s *RoleService) CreateRole(ctx context.Context, tenantID uuid.UUID, req CreateRoleRequest) (Role, error) {
	if req.Name == "" {
		return Role{}, fmt.Errorf("name is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Role{}, fmt.Errorf("begin create role tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var role Role
	err = tx.QueryRowContext(ctx, `
INSERT INTO roles (tenant_id, name, description)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, name, COALESCE(description, ''), created_at
`, tenantID, req.Name, req.Description).Scan(&role.ID, &role.TenantID, &role.Name, &role.Description, &role.CreatedAt)
	if err != nil {
		return Role{}, fmt.Errorf("insert role: %w", err)
	}
	if err := s.replacePermissionsTx(ctx, tx, role.ID, req.Permissions); err != nil {
		return Role{}, err
	}
	if err := tx.Commit(); err != nil {
		return Role{}, fmt.Errorf("commit create role tx: %w", err)
	}
	role.Permissions = req.Permissions
	return role, nil
}

func (s *RoleService) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, name, COALESCE(description, ''), created_at
FROM roles
WHERE tenant_id = $1
ORDER BY name
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	roles := make([]Role, 0)
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.TenantID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		perms, err := s.listRolePermissions(ctx, role.ID)
		if err != nil {
			return nil, err
		}
		role.Permissions = perms
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles: %w", err)
	}
	return roles, nil
}

func (s *RoleService) UpdateRolePermissions(ctx context.Context, tenantID, roleID uuid.UUID, permissions []string) (Role, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Role{}, fmt.Errorf("begin update role permissions tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var role Role
	err = tx.QueryRowContext(ctx, `
SELECT id, tenant_id, name, COALESCE(description, ''), created_at
FROM roles
WHERE id = $1 AND tenant_id = $2
`, roleID, tenantID).Scan(&role.ID, &role.TenantID, &role.Name, &role.Description, &role.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Role{}, sql.ErrNoRows
		}
		return Role{}, fmt.Errorf("load role for permission update: %w", err)
	}

	if err := s.replacePermissionsTx(ctx, tx, roleID, permissions); err != nil {
		return Role{}, err
	}

	if err := tx.Commit(); err != nil {
		return Role{}, fmt.Errorf("commit update role permissions tx: %w", err)
	}

	if s.authz != nil {
		if err := s.authz.invalidatePrincipalsForRole(ctx, roleID); err != nil {
			return Role{}, err
		}
	}
	role.Permissions = permissions
	return role, nil
}

func (s *RoleService) DeleteRole(ctx context.Context, tenantID, roleID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete role tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
SELECT pr.principal_id
FROM principal_roles pr
JOIN roles r ON r.id = pr.role_id
WHERE pr.role_id = $1 AND r.tenant_id = $2
`, roleID, tenantID)
	if err != nil {
		return fmt.Errorf("query principals for role delete invalidation: %w", err)
	}
	principalIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var pid uuid.UUID
		if err := rows.Scan(&pid); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan principal for role delete invalidation: %w", err)
		}
		principalIDs = append(principalIDs, pid)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close principal invalidation rows: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM principal_roles WHERE role_id = $1`, roleID); err != nil {
		return fmt.Errorf("delete principal role assignments: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return fmt.Errorf("delete role permissions: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE id = $1 AND tenant_id = $2`, roleID, tenantID)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete role tx: %w", err)
	}
	if s.authz != nil {
		s.authz.Cache().InvalidateMany(principalIDs)
	}
	return nil
}

func (s *RoleService) replacePermissionsTx(ctx context.Context, tx *sql.Tx, roleID uuid.UUID, permissions []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return fmt.Errorf("clear role permissions: %w", err)
	}
	for _, perm := range permissions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`, roleID, perm); err != nil {
			return fmt.Errorf("insert role permission %q: %w", perm, err)
		}
	}
	return nil
}

func (s *RoleService) listRolePermissions(ctx context.Context, roleID uuid.UUID) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT permission FROM role_permissions WHERE role_id = $1 ORDER BY permission`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	perms := make([]string, 0)
	for rows.Next() {
		var perm string
		if err := rows.Scan(&perm); err != nil {
			return nil, fmt.Errorf("scan role permission: %w", err)
		}
		perms = append(perms, perm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role permissions: %w", err)
	}
	return perms, nil
}
