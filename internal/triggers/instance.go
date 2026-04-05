package triggers

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type TriggerStatus string

const (
	TriggerStarting TriggerStatus = "starting"
	TriggerRunning  TriggerStatus = "running"
	TriggerStopping TriggerStatus = "stopping"
	TriggerStopped  TriggerStatus = "stopped"
	TriggerError    TriggerStatus = "error"
)

type TriggerMetrics struct {
	EventsReceived  int64     `json:"events_received"`
	EventsProcessed int64     `json:"events_processed"`
	EventsFailed    int64     `json:"events_failed"`
	LastEventAt     time.Time `json:"last_event_at"`
}

type TriggerInstanceInfo struct {
	ID            uuid.UUID       `json:"id"`
	TenantID      uuid.UUID       `json:"tenant_id"`
	ChannelID     uuid.UUID       `json:"channel_id"`
	PluginID      string          `json:"plugin_id"`
	PluginVersion string          `json:"plugin_version"`
	Status        TriggerStatus   `json:"status"`
	StartedAt     time.Time       `json:"started_at"`
	StoppedAt     time.Time       `json:"stopped_at"`
	RestartCount  int             `json:"restart_count"`
	Metrics       TriggerMetrics  `json:"metrics"`
	ErrorMessage  string          `json:"error_message"`
	Contract      TriggerContract `json:"contract"`
}

type messageEnvelope struct {
	Message   []byte            `json:"message"`
	Metadata  map[string]string `json:"metadata"`
	MessageID string            `json:"message_id"`
}

type TriggerInstance struct {
	mu            sync.RWMutex
	id            uuid.UUID
	tenantID      uuid.UUID
	channelID     uuid.UUID
	pluginID      string
	pluginVersion string
	contract      TriggerContract
	config        json.RawMessage
	status        TriggerStatus
	cancel        context.CancelFunc
	startedAt     time.Time
	stoppedAt     time.Time
	restartCount  int
	lastBackoff   time.Duration
	errorMessage  string

	eventsReceived  int64
	eventsProcessed int64
	eventsFailed    int64
	lastEventAtNS   int64

	workers      []*worker
	buffer       chan messageEnvelope
	checkpointer Checkpointer
}

func (ti *TriggerInstance) snapshot() *TriggerInstanceInfo {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	_ = len(ti.workers)
	return &TriggerInstanceInfo{
		ID:            ti.id,
		TenantID:      ti.tenantID,
		ChannelID:     ti.channelID,
		PluginID:      ti.pluginID,
		PluginVersion: ti.pluginVersion,
		Status:        ti.status,
		StartedAt:     ti.startedAt,
		StoppedAt:     ti.stoppedAt,
		RestartCount:  ti.restartCount,
		ErrorMessage:  ti.errorMessage,
		Contract:      ti.contract,
		Metrics: TriggerMetrics{
			EventsReceived:  atomic.LoadInt64(&ti.eventsReceived),
			EventsProcessed: atomic.LoadInt64(&ti.eventsProcessed),
			EventsFailed:    atomic.LoadInt64(&ti.eventsFailed),
			LastEventAt:     time.Unix(0, atomic.LoadInt64(&ti.lastEventAtNS)).UTC(),
		},
	}
}

func (ti *TriggerInstance) setStatus(status TriggerStatus, errMsg string) {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	ti.status = status
	now := time.Now().UTC()
	if status == TriggerRunning {
		ti.startedAt = now
		ti.stoppedAt = time.Time{}
		ti.errorMessage = ""
	}
	if status == TriggerStopped || status == TriggerError {
		ti.stoppedAt = now
	}
	if errMsg != "" {
		ti.errorMessage = errMsg
	}
}

func (ti *TriggerInstance) recordReceived() {
	atomic.AddInt64(&ti.eventsReceived, 1)
	atomic.StoreInt64(&ti.lastEventAtNS, time.Now().UTC().UnixNano())
}

func (ti *TriggerInstance) recordProcessed() {
	atomic.AddInt64(&ti.eventsProcessed, 1)
	atomic.StoreInt64(&ti.lastEventAtNS, time.Now().UTC().UnixNano())
}

func (ti *TriggerInstance) recordFailed() {
	atomic.AddInt64(&ti.eventsFailed, 1)
	atomic.StoreInt64(&ti.lastEventAtNS, time.Now().UTC().UnixNano())
}
