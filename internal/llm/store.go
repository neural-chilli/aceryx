package llm

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type InvocationStore interface {
	RecordInvocation(ctx context.Context, inv Invocation) error
	GetMonthlyUsage(ctx context.Context, tenantID uuid.UUID, yearMonth string) (MonthlyUsage, error)
	UpdateMonthlyUsage(ctx context.Context, tenantID uuid.UUID, tokens int, costUSD float64) error
	ListInvocations(ctx context.Context, tenantID uuid.UUID, opts ListOpts) ([]Invocation, error)
	UsageByPurpose(ctx context.Context, tenantID uuid.UUID, since time.Time) ([]PurposeUsage, error)

	ListProviders(ctx context.Context, tenantID uuid.UUID) ([]LLMProviderConfig, error)
	GetProvider(ctx context.Context, tenantID uuid.UUID, configID uuid.UUID) (LLMProviderConfig, error)
	CreateProvider(ctx context.Context, config LLMProviderConfig) (LLMProviderConfig, error)
	UpdateProvider(ctx context.Context, config LLMProviderConfig) (LLMProviderConfig, error)
	DeleteProvider(ctx context.Context, tenantID uuid.UUID, configID uuid.UUID) error
}
