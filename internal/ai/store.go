package ai

import (
	"context"

	"github.com/google/uuid"
)

type TenantComponentStore interface {
	Create(ctx context.Context, tenantID uuid.UUID, def *AIComponentDef, createdBy uuid.UUID) error
	Update(ctx context.Context, tenantID uuid.UUID, def *AIComponentDef) error
	Delete(ctx context.Context, tenantID uuid.UUID, componentID string) error
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*AIComponentDef, error)
}
