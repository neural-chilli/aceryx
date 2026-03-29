package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
		ttl = 60 * time.Second
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

type Service struct {
	db    *sql.DB
	cache *PermissionCache
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, cache: NewPermissionCache(60 * time.Second)}
}

func (s *Service) Authorize(ctx context.Context, principalID uuid.UUID, permission string) error {
	if cp, ok := s.cache.get(principalID); ok {
		if cp.permissions[permission] {
			return nil
		}
		return ErrForbidden
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT rp.permission
FROM principal_roles pr
JOIN role_permissions rp ON rp.role_id = pr.role_id
WHERE pr.principal_id = $1
`, principalID)
	if err != nil {
		return fmt.Errorf("query permissions: %w", err)
	}
	defer rows.Close()

	perms := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return fmt.Errorf("scan permission row: %w", err)
		}
		perms[p] = true
	}
	s.cache.set(principalID, perms)
	if perms[permission] {
		return nil
	}
	return ErrForbidden
}
