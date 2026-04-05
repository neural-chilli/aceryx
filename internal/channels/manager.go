package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/neural-chilli/aceryx/internal/plugins"
	"github.com/neural-chilli/aceryx/internal/triggers"
)

type ChannelRunner interface {
	Start(ctx context.Context) error
	Stop() error
}

type managedChannel struct {
	channel *Channel
	runner  ChannelRunner
}

type ChannelManager struct {
	mu       sync.RWMutex
	channels map[uuid.UUID]*managedChannel

	pipeline    *Pipeline
	store       ChannelStore
	triggers    *triggers.TriggerManager
	drivers     *drivers.DriverRegistry
	secretStore connectors.SecretStore
}

func NewChannelManager(pipeline *Pipeline, store ChannelStore, triggerMgr *triggers.TriggerManager, driverRegistry *drivers.DriverRegistry, secretStore connectors.SecretStore) *ChannelManager {
	return &ChannelManager{pipeline: pipeline, store: store, triggers: triggerMgr, drivers: driverRegistry, secretStore: secretStore, channels: map[uuid.UUID]*managedChannel{}}
}

func (cm *ChannelManager) StartAll(ctx context.Context) error {
	if cm.store == nil {
		return nil
	}
	items, err := cm.store.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, ch := range items {
		if err := cm.Enable(ctx, ch.ID); err != nil {
			return err
		}
	}
	return nil
}

func (cm *ChannelManager) StopAll(ctx context.Context) error {
	_ = ctx
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, item := range cm.channels {
		if item.runner != nil {
			_ = item.runner.Stop()
		}
	}
	cm.channels = map[uuid.UUID]*managedChannel{}
	return nil
}

func (cm *ChannelManager) Enable(ctx context.Context, channelID uuid.UUID) error {
	if cm.store == nil {
		return fmt.Errorf("channel store unavailable")
	}
	ch, err := cm.lookupByID(ctx, channelID)
	if err != nil {
		return err
	}
	if err := cm.store.SetEnabled(ctx, ch.TenantID, ch.ID, true); err != nil {
		return err
	}
	runner, err := cm.buildRunner(ch)
	if err != nil {
		return err
	}
	if runner != nil {
		if err := runner.Start(ctx); err != nil {
			return err
		}
	}
	if ch.Type == ChannelPlugin && cm.triggers != nil {
		ref, err := plugins.ParsePluginRefStrict(ch.PluginRef)
		if err != nil {
			return err
		}
		if err := cm.triggers.Start(ctx, ch.ID, ref, ch.Config); err != nil {
			return err
		}
	}
	cm.mu.Lock()
	cm.channels[ch.ID] = &managedChannel{channel: ch, runner: runner}
	cm.mu.Unlock()
	return nil
}

func (cm *ChannelManager) Disable(ctx context.Context, channelID uuid.UUID) error {
	if cm.store == nil {
		return fmt.Errorf("channel store unavailable")
	}
	ch, err := cm.lookupByID(ctx, channelID)
	if err != nil {
		return err
	}
	if err := cm.store.SetEnabled(ctx, ch.TenantID, ch.ID, false); err != nil {
		return err
	}
	cm.mu.Lock()
	item := cm.channels[channelID]
	delete(cm.channels, channelID)
	cm.mu.Unlock()
	if item != nil && item.runner != nil {
		_ = item.runner.Stop()
	}
	return nil
}

func (cm *ChannelManager) lookupByID(ctx context.Context, channelID uuid.UUID) (*Channel, error) {
	ch, err := cm.store.GetByID(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("channel not found: %w", err)
	}
	return ch, nil
}

func (cm *ChannelManager) buildRunner(ch *Channel) (ChannelRunner, error) {
	if ch == nil {
		return nil, nil
	}
	switch ch.Type {
	case ChannelEmail:
		if cm.drivers == nil {
			return nil, fmt.Errorf("driver registry missing")
		}
		drv, err := cm.drivers.GetIMAP("imap")
		if err != nil {
			return nil, err
		}
		cfg := EmailConfig{}
		_ = json.Unmarshal(ch.Config, &cfg)
		return &EmailChannelRunner{ChannelID: ch.ID, TenantID: ch.TenantID, Config: cfg, Pipeline: cm.pipeline, IMAP: drv, SecretStore: cm.secretStore}, nil
	case ChannelFileDrop:
		cfg := FileDropConfig{}
		_ = json.Unmarshal(ch.Config, &cfg)
		return &FileDropChannelRunner{ChannelID: ch.ID, TenantID: ch.TenantID, Config: cfg, Pipeline: cm.pipeline}, nil
	default:
		return nil, nil
	}
}
