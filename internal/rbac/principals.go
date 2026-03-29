package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type PrincipalService struct {
	db    *sql.DB
	authz *Service
}

func NewPrincipalService(db *sql.DB, authz *Service) *PrincipalService {
	return &PrincipalService{db: db, authz: authz}
}

func (s *PrincipalService) CreatePrincipal(ctx context.Context, tenantID uuid.UUID, req CreatePrincipalRequest) (Principal, string, error) {
	if req.Name == "" {
		return Principal{}, "", fmt.Errorf("name is required")
	}
	if req.Type != "human" && req.Type != "agent" {
		return Principal{}, "", fmt.Errorf("type must be human or agent")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Principal{}, "", fmt.Errorf("begin create principal tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var passwordHash *string
	apiKeyPlaintext := ""
	apiKeyHash := ""
	if req.Type == "human" {
		if req.Email == "" || req.Password == "" {
			return Principal{}, "", fmt.Errorf("email and password are required for human principals")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			return Principal{}, "", fmt.Errorf("hash principal password: %w", err)
		}
		h := string(hash)
		passwordHash = &h
	} else {
		key, hash, err := GenerateAPIKey()
		if err != nil {
			return Principal{}, "", fmt.Errorf("generate api key: %w", err)
		}
		apiKeyPlaintext = key
		apiKeyHash = hash
	}

	var p Principal
	err = tx.QueryRowContext(ctx, `
INSERT INTO principals (tenant_id, type, name, email, password_hash, api_key_hash, status, metadata)
VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), 'active', COALESCE($7::jsonb, '{}'::jsonb))
RETURNING id, tenant_id, type, name, COALESCE(email, ''), status, metadata, created_at
`, tenantID, req.Type, req.Name, strings.ToLower(strings.TrimSpace(req.Email)), passwordHash, apiKeyHash, string(req.Metadata)).Scan(&p.ID, &p.TenantID, &p.Type, &p.Name, &p.Email, &p.Status, &p.Metadata, &p.CreatedAt)
	if err != nil {
		return Principal{}, "", fmt.Errorf("insert principal: %w", err)
	}

	if err := s.replacePrincipalRolesTx(ctx, tx, tenantID, p.ID, req.Roles); err != nil {
		return Principal{}, "", err
	}

	if err := tx.Commit(); err != nil {
		return Principal{}, "", fmt.Errorf("commit create principal tx: %w", err)
	}

	if s.authz != nil {
		s.authz.Cache().Invalidate(p.ID)
	}
	roles, _ := listPrincipalRoleNames(ctx, s.db, p.ID)
	p.Roles = roles
	return p, apiKeyPlaintext, nil
}

func (s *PrincipalService) ListPrincipals(ctx context.Context, tenantID uuid.UUID) ([]Principal, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, type, name, COALESCE(email, ''), status, COALESCE(metadata, '{}'::jsonb), created_at
FROM principals
WHERE tenant_id = $1
ORDER BY created_at DESC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list principals: %w", err)
	}
	defer rows.Close()

	out := make([]Principal, 0)
	for rows.Next() {
		var p Principal
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Type, &p.Name, &p.Email, &p.Status, &p.Metadata, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan principal: %w", err)
		}
		roles, err := listPrincipalRoleNames(ctx, s.db, p.ID)
		if err != nil {
			return nil, err
		}
		p.Roles = roles
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate principals: %w", err)
	}
	return out, nil
}

func (s *PrincipalService) UpdatePrincipal(ctx context.Context, tenantID, principalID uuid.UUID, req UpdatePrincipalRequest) (Principal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Principal{}, fmt.Errorf("begin update principal tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentType string
	err = tx.QueryRowContext(ctx, `SELECT type FROM principals WHERE id = $1 AND tenant_id = $2`, principalID, tenantID).Scan(&currentType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Principal{}, sql.ErrNoRows
		}
		return Principal{}, fmt.Errorf("load principal for update: %w", err)
	}

	if req.Name != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE principals SET name = $3 WHERE id = $1 AND tenant_id = $2`, principalID, tenantID, *req.Name); err != nil {
			return Principal{}, fmt.Errorf("update principal name: %w", err)
		}
	}
	if req.Email != nil {
		if currentType != "human" {
			return Principal{}, fmt.Errorf("only human principals can have email")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE principals SET email = NULLIF($3, '') WHERE id = $1 AND tenant_id = $2`, principalID, tenantID, strings.ToLower(strings.TrimSpace(*req.Email))); err != nil {
			return Principal{}, fmt.Errorf("update principal email: %w", err)
		}
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "disabled" {
			return Principal{}, fmt.Errorf("invalid status")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE principals SET status = $3 WHERE id = $1 AND tenant_id = $2`, principalID, tenantID, *req.Status); err != nil {
			return Principal{}, fmt.Errorf("update principal status: %w", err)
		}
		if *req.Status == "disabled" {
			if _, err := tx.ExecContext(ctx, `
DELETE FROM sessions s
USING principals p
WHERE s.principal_id = p.id
  AND p.id = $1
  AND p.tenant_id = $2
`, principalID, tenantID); err != nil {
				return Principal{}, fmt.Errorf("invalidate sessions for disabled principal: %w", err)
			}
		}
	}
	if req.Roles != nil {
		if err := s.replacePrincipalRolesTx(ctx, tx, tenantID, principalID, req.Roles); err != nil {
			return Principal{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Principal{}, fmt.Errorf("commit update principal tx: %w", err)
	}

	if s.authz != nil {
		s.authz.Cache().Invalidate(principalID)
	}
	p, err := s.GetPrincipal(ctx, tenantID, principalID)
	if err != nil {
		return Principal{}, err
	}
	return p, nil
}

func (s *PrincipalService) DisablePrincipal(ctx context.Context, tenantID, principalID uuid.UUID) error {
	_, err := s.UpdatePrincipal(ctx, tenantID, principalID, UpdatePrincipalRequest{Status: ptrString("disabled")})
	return err
}

func (s *PrincipalService) GetPrincipal(ctx context.Context, tenantID, principalID uuid.UUID) (Principal, error) {
	var p Principal
	err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, type, name, COALESCE(email, ''), status, COALESCE(metadata, '{}'::jsonb), created_at
FROM principals
WHERE id = $1 AND tenant_id = $2
`, principalID, tenantID).Scan(&p.ID, &p.TenantID, &p.Type, &p.Name, &p.Email, &p.Status, &p.Metadata, &p.CreatedAt)
	if err != nil {
		return Principal{}, err
	}
	roles, err := listPrincipalRoleNames(ctx, s.db, p.ID)
	if err != nil {
		return Principal{}, err
	}
	p.Roles = roles
	return p, nil
}

func (s *PrincipalService) replacePrincipalRolesTx(ctx context.Context, tx *sql.Tx, tenantID, principalID uuid.UUID, roleNames []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM principal_roles WHERE principal_id = $1`, principalID); err != nil {
		return fmt.Errorf("clear principal roles: %w", err)
	}
	for _, roleName := range roleNames {
		var roleID uuid.UUID
		err := tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE tenant_id = $1 AND name = $2`, tenantID, roleName).Scan(&roleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("role %q not found", roleName)
			}
			return fmt.Errorf("resolve role %q: %w", roleName, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO principal_roles (principal_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, principalID, roleID); err != nil {
			return fmt.Errorf("assign role %q: %w", roleName, err)
		}
	}
	return nil
}

func listPrincipalRoleNames(ctx context.Context, db *sql.DB, principalID uuid.UUID) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT r.name
FROM principal_roles pr
JOIN roles r ON r.id = pr.role_id
WHERE pr.principal_id = $1
ORDER BY r.name
`, principalID)
	if err != nil {
		return nil, fmt.Errorf("query principal role names: %w", err)
	}
	defer rows.Close()

	roles := make([]string, 0)
	for rows.Next() {
		var roleName string
		if err := rows.Scan(&roleName); err != nil {
			return nil, fmt.Errorf("scan principal role name: %w", err)
		}
		roles = append(roles, roleName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate principal role names: %w", err)
	}
	return roles, nil
}

func ptrString(v string) *string {
	return &v
}
