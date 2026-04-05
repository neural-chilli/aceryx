package triggers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type TriggerStore interface {
	Create(ctx context.Context, instance *TriggerInstanceRecord) error
	Update(ctx context.Context, instance *TriggerInstanceRecord) error
	Get(ctx context.Context, id uuid.UUID) (*TriggerInstanceRecord, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*TriggerInstanceRecord, error)
	ListByChannel(ctx context.Context, channelID uuid.UUID) ([]*TriggerInstanceRecord, error)
}

type TriggerInstanceRecord struct {
	ID              uuid.UUID       `json:"id"`
	TenantID        uuid.UUID       `json:"tenant_id"`
	ChannelID       uuid.UUID       `json:"channel_id"`
	PluginID        string          `json:"plugin_id"`
	PluginVersion   string          `json:"plugin_version"`
	Status          string          `json:"status"`
	StartedAt       *time.Time      `json:"started_at"`
	StoppedAt       *time.Time      `json:"stopped_at"`
	EventsReceived  int64           `json:"events_received"`
	EventsProcessed int64           `json:"events_processed"`
	EventsFailed    int64           `json:"events_failed"`
	LastEventAt     *time.Time      `json:"last_event_at"`
	ErrorMessage    string          `json:"error_message"`
	RestartCount    int             `json:"restart_count"`
	Config          json.RawMessage `json:"config"`
}
