package triggers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/neural-chilli/aceryx/internal/plugins"
)

var ErrTriggerInstanceNotFound = errors.New("trigger instance not found")

type TriggerManager struct {
	mu        sync.RWMutex
	instances map[uuid.UUID]*TriggerInstance

	runtime  plugins.PluginRuntime
	drivers  *drivers.DriverRegistry
	pipeline ChannelPipeline
	store    TriggerStore
	config   TriggerManagerConfig
}

type TriggerManagerConfig struct {
	MaxRestartBackoff   time.Duration
	HealthyRunDuration  time.Duration
	GracefulStopTimeout time.Duration
	DefaultBufferSize   int
	DefaultWorkerCount  int
}

func NewTriggerManager(
	runtime plugins.PluginRuntime,
	_ any,
	drivers *drivers.DriverRegistry,
	pipeline ChannelPipeline,
	store TriggerStore,
	config TriggerManagerConfig,
) *TriggerManager {
	if config.MaxRestartBackoff <= 0 {
		config.MaxRestartBackoff = 60 * time.Second
	}
	if config.HealthyRunDuration <= 0 {
		config.HealthyRunDuration = 60 * time.Second
	}
	if config.GracefulStopTimeout <= 0 {
		config.GracefulStopTimeout = 30 * time.Second
	}
	if config.DefaultBufferSize <= 0 {
		config.DefaultBufferSize = 100
	}
	if config.DefaultWorkerCount <= 0 {
		config.DefaultWorkerCount = 4
	}
	if pipeline == nil {
		pipeline = NewStubChannelPipeline(nil)
	}
	return &TriggerManager{
		instances: make(map[uuid.UUID]*TriggerInstance),
		runtime:   runtime,
		drivers:   drivers,
		pipeline:  pipeline,
		store:     store,
		config:    config,
	}
}

func (tm *TriggerManager) Start(ctx context.Context, channelID uuid.UUID, pluginRef plugins.PluginRef, config json.RawMessage) error {
	if tm.runtime == nil {
		return fmt.Errorf("plugin runtime not configured")
	}
	if channelID == uuid.Nil {
		return fmt.Errorf("channel id is required")
	}
	plugin, err := tm.runtime.Get(pluginRef)
	if err != nil {
		return err
	}
	if plugin.Type != plugins.TriggerPlugin {
		return fmt.Errorf("plugin %s@%s is not a trigger plugin", plugin.ID, plugin.Version)
	}
	contract, err := ParseTriggerContract(plugin.Manifest.TriggerContract)
	if err != nil {
		return fmt.Errorf("invalid trigger contract: %w", err)
	}
	meta, err := triggerConfigMetaFromJSON(config)
	if err != nil {
		return err
	}
	tenantID := meta.TenantID
	if contract.Delivery == DeliveryExactlyOnce {
		if meta.DriverID == "" {
			return fmt.Errorf("exactly_once delivery requires driver_id in trigger config")
		}
		if tm.drivers == nil {
			return fmt.Errorf("exactly_once delivery cannot be validated: driver registry not configured")
		}
		driver, err := tm.drivers.GetQueue(meta.DriverID)
		if err != nil {
			return err
		}
		if err := NewDeliveryHandler(contract.Delivery, driver, tm.pipeline, tenantID).ValidateStartup(); err != nil {
			return err
		}
	}

	inst := &TriggerInstance{
		id:            uuid.New(),
		tenantID:      tenantID,
		channelID:     channelID,
		pluginID:      plugin.ID,
		pluginVersion: plugin.Version,
		contract:      contract,
		config:        config,
		status:        TriggerStarting,
	}
	if contract.Concurrency == ConcurrencyParallel {
		inst.buffer = make(chan messageEnvelope, tm.config.DefaultBufferSize)
	}

	if err := tm.runtime.StartTrigger(plugins.PluginRef{ID: plugin.ID, Version: plugin.Version}, plugins.TriggerConfig{
		TenantID: tenantID,
		Config:   config,
	}); err != nil {
		inst.setStatus(TriggerError, err.Error())
		return err
	}
	inst.setStatus(TriggerRunning, "")

	tm.mu.Lock()
	tm.instances[inst.id] = inst
	tm.mu.Unlock()

	tm.persist(ctx, inst)
	return nil
}

func (tm *TriggerManager) Stop(ctx context.Context, instanceID uuid.UUID) error {
	if tm.runtime == nil {
		return fmt.Errorf("plugin runtime not configured")
	}
	inst, err := tm.getInstance(instanceID)
	if err != nil {
		return err
	}
	inst.setStatus(TriggerStopping, "")
	if inst.cancel != nil {
		inst.cancel()
	}

	stopCtx, cancel := context.WithTimeout(ctx, tm.config.GracefulStopTimeout)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- tm.runtime.StopTrigger(plugins.PluginRef{ID: inst.pluginID, Version: inst.pluginVersion})
	}()

	select {
	case err := <-errCh:
		if err != nil {
			inst.setStatus(TriggerError, err.Error())
			tm.persist(ctx, inst)
			return err
		}
	case <-stopCtx.Done():
		inst.setStatus(TriggerStopped, "graceful stop timeout; forced stop")
		tm.persist(ctx, inst)
		return nil
	}
	inst.setStatus(TriggerStopped, "")
	tm.persist(ctx, inst)
	return nil
}

func (tm *TriggerManager) Restart(ctx context.Context, instanceID uuid.UUID) error {
	inst, err := tm.getInstance(instanceID)
	if err != nil {
		return err
	}
	if err := tm.Stop(ctx, instanceID); err != nil {
		return err
	}
	meta, err := triggerConfigMetaFromJSON(inst.config)
	if err != nil {
		return err
	}
	if err := tm.runtime.StartTrigger(plugins.PluginRef{ID: inst.pluginID, Version: inst.pluginVersion}, plugins.TriggerConfig{
		TenantID: meta.TenantID,
		Config:   inst.config,
	}); err != nil {
		inst.setStatus(TriggerError, err.Error())
		tm.persist(ctx, inst)
		return err
	}
	inst.mu.Lock()
	inst.restartCount++
	inst.status = TriggerRunning
	inst.startedAt = time.Now().UTC()
	inst.stoppedAt = time.Time{}
	inst.mu.Unlock()
	tm.persist(ctx, inst)
	return nil
}

func (tm *TriggerManager) StopAll(ctx context.Context) error {
	items := tm.List()
	for _, item := range items {
		if err := tm.Stop(ctx, item.ID); err != nil {
			return err
		}
	}
	return nil
}

func (tm *TriggerManager) List() []*TriggerInstanceInfo {
	tm.mu.RLock()
	out := make([]*TriggerInstanceInfo, 0, len(tm.instances))
	for _, instance := range tm.instances {
		out = append(out, instance.snapshot())
	}
	tm.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

func (tm *TriggerManager) Get(instanceID uuid.UUID) (*TriggerInstanceInfo, error) {
	instance, err := tm.getInstance(instanceID)
	if err != nil {
		return nil, err
	}
	return instance.snapshot(), nil
}

func (tm *TriggerManager) ResetCheckpoints(ctx context.Context, instanceID uuid.UUID) error {
	inst, err := tm.getInstance(instanceID)
	if err != nil {
		return err
	}
	inst.mu.RLock()
	status := inst.status
	cp := inst.checkpointer
	inst.mu.RUnlock()

	if status != TriggerStopped {
		return fmt.Errorf("trigger must be stopped before resetting checkpoints")
	}
	if cp == nil {
		return nil
	}
	return cp.DeleteAll(ctx, instanceID)
}

func (tm *TriggerManager) ListCheckpoints(ctx context.Context, instanceID uuid.UUID) ([]CheckpointRecord, error) {
	inst, err := tm.getInstance(instanceID)
	if err != nil {
		return nil, err
	}
	inst.mu.RLock()
	cp := inst.checkpointer
	inst.mu.RUnlock()
	reader, ok := cp.(CheckpointReader)
	if !ok || reader == nil {
		return []CheckpointRecord{}, nil
	}
	return reader.List(ctx, instanceID)
}

func (tm *TriggerManager) getInstance(instanceID uuid.UUID) (*TriggerInstance, error) {
	tm.mu.RLock()
	inst := tm.instances[instanceID]
	tm.mu.RUnlock()
	if inst == nil {
		return nil, ErrTriggerInstanceNotFound
	}
	return inst, nil
}

func (tm *TriggerManager) persist(ctx context.Context, inst *TriggerInstance) {
	if tm.store == nil || inst == nil {
		return
	}
	info := inst.snapshot()
	rec := &TriggerInstanceRecord{
		ID:              info.ID,
		TenantID:        info.TenantID,
		ChannelID:       info.ChannelID,
		PluginID:        info.PluginID,
		PluginVersion:   info.PluginVersion,
		Status:          string(info.Status),
		ErrorMessage:    info.ErrorMessage,
		RestartCount:    info.RestartCount,
		EventsReceived:  info.Metrics.EventsReceived,
		EventsProcessed: info.Metrics.EventsProcessed,
		EventsFailed:    info.Metrics.EventsFailed,
		Config:          inst.config,
	}
	if !info.StartedAt.IsZero() {
		t := info.StartedAt
		rec.StartedAt = &t
	}
	if !info.StoppedAt.IsZero() {
		t := info.StoppedAt
		rec.StoppedAt = &t
	}
	if !info.Metrics.LastEventAt.IsZero() {
		t := info.Metrics.LastEventAt
		rec.LastEventAt = &t
	}
	if err := tm.store.Update(ctx, rec); err != nil {
		_ = tm.store.Create(ctx, rec)
	}
}

func (tm *TriggerManager) restartWithBackoff(ctx context.Context, instanceID uuid.UUID) {
	inst, err := tm.getInstance(instanceID)
	if err != nil {
		return
	}
	inst.mu.Lock()
	next := inst.lastBackoff
	if next <= 0 {
		next = time.Second
	}
	if next > tm.config.MaxRestartBackoff {
		next = tm.config.MaxRestartBackoff
	} else if inst.lastBackoff > 0 {
		next *= 2
		if next > tm.config.MaxRestartBackoff {
			next = tm.config.MaxRestartBackoff
		}
	}
	inst.lastBackoff = next
	inst.restartCount++
	inst.mu.Unlock()

	t := time.NewTimer(next)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return
	case <-t.C:
		_ = tm.Restart(ctx, instanceID)
	}
}

func (tm *TriggerManager) maybeResetBackoff(instanceID uuid.UUID) {
	inst, err := tm.getInstance(instanceID)
	if err != nil {
		return
	}
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.status == TriggerRunning && !inst.startedAt.IsZero() && time.Since(inst.startedAt) >= tm.config.HealthyRunDuration {
		inst.lastBackoff = 0
	}
}

type triggerConfigMeta struct {
	TenantID uuid.UUID
	DriverID string
}

func triggerConfigMetaFromJSON(config json.RawMessage) (triggerConfigMeta, error) {
	if len(config) == 0 {
		return triggerConfigMeta{}, fmt.Errorf("trigger config must include tenant_id")
	}
	var payload struct {
		TenantID string `json:"tenant_id"`
		DriverID string `json:"driver_id"`
	}
	if err := json.Unmarshal(config, &payload); err != nil {
		return triggerConfigMeta{}, fmt.Errorf("decode trigger config: %w", err)
	}
	if payload.TenantID == "" {
		return triggerConfigMeta{}, fmt.Errorf("trigger config must include tenant_id")
	}
	tenantID, err := uuid.Parse(payload.TenantID)
	if err != nil {
		return triggerConfigMeta{}, fmt.Errorf("invalid tenant_id: %w", err)
	}
	return triggerConfigMeta{TenantID: tenantID, DriverID: payload.DriverID}, nil
}
