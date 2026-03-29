package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrForbidden = errors.New("forbidden")

type cachedPermissions struct {
	permissions map[string]bool
	loadedAt    time.Time
}

type PermissionCache struct {
	mu    sync.RWMutex
	cache map[uuid.UUID]*cachedPermissions
	ttl   time.Duration
}

func NewPermissionCache(ttl time.Duration) *PermissionCache {
	if ttl <= 0 {
		ttl = DefaultPermissionTTL
	}
	return &PermissionCache{cache: map[uuid.UUID]*cachedPermissions{}, ttl: ttl}
}

func (pc *PermissionCache) get(principalID uuid.UUID) (*cachedPermissions, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	cp, ok := pc.cache[principalID]
	if !ok {
		return nil, false
	}
	if time.Since(cp.loadedAt) > pc.ttl {
		return nil, false
	}
	return cp, true
}

func (pc *PermissionCache) set(principalID uuid.UUID, perms map[string]bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.cache[principalID] = &cachedPermissions{permissions: perms, loadedAt: time.Now()}
}

func (pc *PermissionCache) Invalidate(principalID uuid.UUID) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	delete(pc.cache, principalID)
}

func (pc *PermissionCache) InvalidateMany(principalIDs []uuid.UUID) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for _, id := range principalIDs {
		delete(pc.cache, id)
	}
}

func (pc *PermissionCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.cache = map[uuid.UUID]*cachedPermissions{}
}

type permissionLoader func(ctx context.Context, principalID uuid.UUID) (map[string]bool, error)

type Service struct {
	db     *sql.DB
	cache  *PermissionCache
	loader permissionLoader
}

func NewService(db *sql.DB) *Service {
	s := &Service{db: db, cache: NewPermissionCache(DefaultPermissionTTL)}
	s.loader = s.loadPermissions
	return s
}

func (s *Service) Cache() *PermissionCache {
	return s.cache
}

func Authorize(ctx context.Context, db *sql.DB, principalID uuid.UUID, permission string) error {
	return NewService(db).Authorize(ctx, principalID, permission)
}

func (s *Service) Authorize(ctx context.Context, principalID uuid.UUID, permission string) error {
	if cp, ok := s.cache.get(principalID); ok {
		if hasPermission(cp.permissions, permission) {
			return nil
		}
		return ErrForbidden
	}

	perms, err := s.loader(ctx, principalID)
	if err != nil {
		return err
	}
	s.cache.set(principalID, perms)
	if hasPermission(perms, permission) {
		return nil
	}
	return ErrForbidden
}

func (s *Service) loadPermissions(ctx context.Context, principalID uuid.UUID) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT rp.permission
FROM principals p
JOIN principal_roles pr ON pr.principal_id = p.id
JOIN roles r ON r.id = pr.role_id AND r.tenant_id = p.tenant_id
JOIN role_permissions rp ON rp.role_id = pr.role_id
WHERE p.id = $1
  AND p.status = 'active'
`, principalID)
	if err != nil {
		return nil, fmt.Errorf("query permissions: %w", err)
	}
	defer rows.Close()

	perms := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan permission row: %w", err)
		}
		perms[p] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permission rows: %w", err)
	}
	return perms, nil
}

func hasPermission(perms map[string]bool, requested string) bool {
	if perms[requested] || perms["*"] {
		return true
	}
	if idx := strings.IndexByte(requested, ':'); idx > 0 {
		if perms[requested[:idx]+":*"] {
			return true
		}
	}
	return false
}

func (s *Service) invalidatePrincipalsForRole(ctx context.Context, roleID uuid.UUID) error {
	rows, err := s.db.QueryContext(ctx, `SELECT principal_id FROM principal_roles WHERE role_id = $1`, roleID)
	if err != nil {
		return fmt.Errorf("query principals for role invalidation: %w", err)
	}
	defer rows.Close()

	principalIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var principalID uuid.UUID
		if err := rows.Scan(&principalID); err != nil {
			return fmt.Errorf("scan principal for invalidation: %w", err)
		}
		principalIDs = append(principalIDs, principalID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate principals for invalidation: %w", err)
	}
	s.cache.InvalidateMany(principalIDs)
	return nil
}
