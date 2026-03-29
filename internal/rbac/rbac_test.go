package rbac

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAuthorize_PrincipalWithPermissionAllowed(t *testing.T) {
	principalID := uuid.New()
	svc := &Service{cache: NewPermissionCache(time.Minute)}
	svc.loader = func(_ context.Context, id uuid.UUID) (map[string]bool, error) {
		if id != principalID {
			return nil, ErrForbidden
		}
		return map[string]bool{"cases:read": true}, nil
	}

	if err := svc.Authorize(context.Background(), principalID, "cases:read"); err != nil {
		t.Fatalf("expected allow, got error: %v", err)
	}
}

func TestAuthorize_PrincipalWithoutPermissionDenied(t *testing.T) {
	svc := &Service{cache: NewPermissionCache(time.Minute)}
	svc.loader = func(_ context.Context, _ uuid.UUID) (map[string]bool, error) {
		return map[string]bool{"cases:update": true}, nil
	}

	if err := svc.Authorize(context.Background(), uuid.New(), "cases:read"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
}

func TestAuthorize_DisabledPrincipalDenied(t *testing.T) {
	svc := &Service{cache: NewPermissionCache(time.Minute)}
	svc.loader = func(_ context.Context, _ uuid.UUID) (map[string]bool, error) {
		return map[string]bool{}, nil
	}

	if err := svc.Authorize(context.Background(), uuid.New(), "cases:read"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for disabled/no-permissions principal, got: %v", err)
	}
}

func TestAuthorize_PermissionViaMultipleRolesAllowed(t *testing.T) {
	svc := &Service{cache: NewPermissionCache(time.Minute)}
	svc.loader = func(_ context.Context, _ uuid.UUID) (map[string]bool, error) {
		return map[string]bool{"workflows:*": true, "cases:read": true}, nil
	}

	if err := svc.Authorize(context.Background(), uuid.New(), "workflows:deploy"); err != nil {
		t.Fatalf("expected wildcard workflows permission to allow, got: %v", err)
	}
}

func TestPermissionCache_HitAndInvalidation(t *testing.T) {
	principalID := uuid.New()
	calls := 0
	svc := &Service{cache: NewPermissionCache(time.Minute)}
	svc.loader = func(_ context.Context, _ uuid.UUID) (map[string]bool, error) {
		calls++
		return map[string]bool{"cases:read": true}, nil
	}

	if err := svc.Authorize(context.Background(), principalID, "cases:read"); err != nil {
		t.Fatalf("authorize first call: %v", err)
	}
	if err := svc.Authorize(context.Background(), principalID, "cases:read"); err != nil {
		t.Fatalf("authorize second call: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected loader to be called once with cache hit, got %d", calls)
	}

	svc.Cache().Invalidate(principalID)
	if err := svc.Authorize(context.Background(), principalID, "cases:read"); err != nil {
		t.Fatalf("authorize after invalidation: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected loader called twice after invalidation, got %d", calls)
	}
}
