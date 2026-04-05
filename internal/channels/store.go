package channels

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ChannelStore interface {
	Create(ctx context.Context, channel *Channel) error
	Update(ctx context.Context, channel *Channel) error
	Get(ctx context.Context, tenantID, channelID uuid.UUID) (*Channel, error)
	GetByID(ctx context.Context, channelID uuid.UUID) (*Channel, error)
	List(ctx context.Context, tenantID uuid.UUID) ([]*Channel, error)
	ListEnabled(ctx context.Context) ([]*Channel, error)
	SoftDelete(ctx context.Context, tenantID, channelID uuid.UUID) error
	SetEnabled(ctx context.Context, tenantID, channelID uuid.UUID, enabled bool) error
	ListEvents(ctx context.Context, tenantID, channelID uuid.UUID, limit, offset int) ([]*ChannelEvent, error)
	RecordFailedEvent(ctx context.Context, event *ChannelEvent) (uuid.UUID, error)
	WithTx(ctx context.Context, fn func(txCtx context.Context, tx TxStore) error) error
}

type TxStore interface {
	GetChannel(ctx context.Context, tenantID, channelID uuid.UUID) (*Channel, error)
	FindRecentEvents(ctx context.Context, tenantID, channelID uuid.UUID, since time.Time) ([]*ChannelEvent, error)
	FindCaseByFields(ctx context.Context, tenantID, caseTypeID uuid.UUID, fields []string, inbound json.RawMessage) (*uuid.UUID, error)
	CreateCase(ctx context.Context, in CreateOrUpdateCaseInput) (uuid.UUID, error)
	UpdateCaseData(ctx context.Context, tenantID, caseID uuid.UUID, patch map[string]any) error
	RecordEvent(ctx context.Context, event *ChannelEvent) (uuid.UUID, error)
}

type CreateOrUpdateCaseInput struct {
	TenantID   uuid.UUID
	ChannelID  uuid.UUID
	CaseTypeID uuid.UUID
	WorkflowID *uuid.UUID
	Data       map[string]any
	ActorID    uuid.UUID
}
