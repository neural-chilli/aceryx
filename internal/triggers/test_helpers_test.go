package triggers

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/neural-chilli/aceryx/internal/plugins"
)

type mockRuntime struct {
	mu         sync.Mutex
	plugin     *plugins.Plugin
	startCalls int
	stopCalls  int
	startErr   error
	stopErr    error
}

func (m *mockRuntime) LoadAll(string, plugins.LicenceKey) error                 { return nil }
func (m *mockRuntime) Load(string, plugins.LicenceKey) (*plugins.Plugin, error) { return nil, nil }
func (m *mockRuntime) Unload(plugins.PluginRef) error                           { return nil }
func (m *mockRuntime) Reload(plugins.PluginRef) error                           { return nil }
func (m *mockRuntime) ExecuteStep(context.Context, plugins.PluginRef, plugins.StepInput) (plugins.StepResult, error) {
	return plugins.StepResult{}, nil
}
func (m *mockRuntime) StartTrigger(plugins.PluginRef, plugins.TriggerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalls++
	return m.startErr
}
func (m *mockRuntime) StopTrigger(plugins.PluginRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls++
	return m.stopErr
}
func (m *mockRuntime) List() []*plugins.Plugin { return nil }
func (m *mockRuntime) Get(plugins.PluginRef) (*plugins.Plugin, error) {
	if m.plugin == nil {
		return nil, errors.New("not found")
	}
	return m.plugin, nil
}
func (m *mockRuntime) ListVersions(string) ([]*plugins.Plugin, error) { return nil, nil }
func (m *mockRuntime) StepPalette() []plugins.PaletteCategory         { return nil }
func (m *mockRuntime) ToolPalette() []plugins.PaletteCategory         { return nil }
func (m *mockRuntime) Search(string) []*plugins.Plugin                { return nil }
func (m *mockRuntime) LastSchemaChange(string) (plugins.SchemaChangeReport, bool) {
	return plugins.SchemaChangeReport{}, false
}

type mockQueueDriver struct {
	id   string
	ack  int
	nack int
	mu   sync.Mutex
}

func (m *mockQueueDriver) ID() string                                         { return m.id }
func (m *mockQueueDriver) DisplayName() string                                { return m.id }
func (m *mockQueueDriver) Connect(context.Context, drivers.QueueConfig) error { return nil }
func (m *mockQueueDriver) Publish(context.Context, string, []byte, map[string]string) error {
	return nil
}
func (m *mockQueueDriver) Consume(context.Context, string) ([]byte, map[string]string, string, error) {
	return nil, nil, "", nil
}
func (m *mockQueueDriver) Ack(context.Context, string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ack++
	return nil
}
func (m *mockQueueDriver) Nack(context.Context, string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nack++
	return nil
}
func (m *mockQueueDriver) Close() error { return nil }

type mockExactlyOnceDriver struct{ mockQueueDriver }

func (m *mockExactlyOnceDriver) SupportsExactlyOnce() bool { return true }

type mockPipeline struct {
	fn func(ctx context.Context, req PipelineRequest) (PipelineResult, error)
}

func (m *mockPipeline) Process(ctx context.Context, req PipelineRequest) (PipelineResult, error) {
	if m.fn != nil {
		return m.fn(ctx, req)
	}
	return PipelineResult{CaseID: uuid.New(), EventID: uuid.New()}, nil
}

type memoryStore struct {
	mu    sync.Mutex
	items map[uuid.UUID]*TriggerInstanceRecord
}

func newMemoryStore() *memoryStore {
	return &memoryStore{items: map[uuid.UUID]*TriggerInstanceRecord{}}
}

func (m *memoryStore) Create(_ context.Context, instance *TriggerInstanceRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *instance
	m.items[instance.ID] = &cp
	return nil
}
func (m *memoryStore) Update(ctx context.Context, instance *TriggerInstanceRecord) error {
	return m.Create(ctx, instance)
}
func (m *memoryStore) Get(_ context.Context, id uuid.UUID) (*TriggerInstanceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil {
		return nil, errors.New("not found")
	}
	cp := *item
	return &cp, nil
}
func (m *memoryStore) ListByTenant(_ context.Context, tenantID uuid.UUID) ([]*TriggerInstanceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*TriggerInstanceRecord, 0)
	for _, item := range m.items {
		if item.TenantID == tenantID {
			cp := *item
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (m *memoryStore) ListByChannel(_ context.Context, channelID uuid.UUID) ([]*TriggerInstanceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*TriggerInstanceRecord, 0)
	for _, item := range m.items {
		if item.ChannelID == channelID {
			cp := *item
			out = append(out, &cp)
		}
	}
	return out, nil
}

type memoryCheckpointer struct {
	mu      sync.Mutex
	entries map[string]string
}

func newMemoryCheckpointer() *memoryCheckpointer {
	return &memoryCheckpointer{entries: map[string]string{}}
}

func (m *memoryCheckpointer) Save(_ context.Context, _ uuid.UUID, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = value
	return nil
}
func (m *memoryCheckpointer) Load(_ context.Context, _ uuid.UUID, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[key], nil
}
func (m *memoryCheckpointer) DeleteAll(_ context.Context, _ uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = map[string]string{}
	return nil
}
func (m *memoryCheckpointer) List(_ context.Context, _ uuid.UUID) ([]CheckpointRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CheckpointRecord, 0, len(m.entries))
	for k, v := range m.entries {
		out = append(out, CheckpointRecord{Key: k, Value: v})
	}
	return out, nil
}

func triggerPluginForTests(contract *plugins.TriggerContract) *plugins.Plugin {
	return &plugins.Plugin{
		ID:      "test-trigger",
		Version: "1.0.0",
		Type:    plugins.TriggerPlugin,
		Manifest: plugins.PluginManifest{
			TriggerContract: contract,
		},
	}
}

func triggerConfigJSON(tenantID uuid.UUID, driverID string) json.RawMessage {
	payload := map[string]any{"tenant_id": tenantID.String()}
	if driverID != "" {
		payload["driver_id"] = driverID
	}
	b, _ := json.Marshal(payload)
	return b
}

func waitUntil(t time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(t)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fn()
}
