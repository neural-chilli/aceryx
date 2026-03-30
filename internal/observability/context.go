package observability

import (
	"context"

	"github.com/google/uuid"
)

const CorrelationHeader = "X-Correlation-ID"

type correlationIDKey struct{}
type tenantIDKey struct{}
type principalIDKey struct{}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, id)
}

func CorrelationIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(correlationIDKey{}).(string)
	return v
}

func WithTenantID(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantIDKey{}, tenantID)
}

func TenantIDFromContext(ctx context.Context) string {
	v, ok := ctx.Value(tenantIDKey{}).(uuid.UUID)
	if !ok {
		return ""
	}
	return v.String()
}

func WithPrincipalID(ctx context.Context, principalID uuid.UUID) context.Context {
	return context.WithValue(ctx, principalIDKey{}, principalID)
}

func PrincipalIDFromContext(ctx context.Context) string {
	v, ok := ctx.Value(principalIDKey{}).(uuid.UUID)
	if !ok {
		return ""
	}
	return v.String()
}
