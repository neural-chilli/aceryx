package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

var (
	ErrPluginNotLoaded = errors.New("plugin not loaded")
)

type RuntimeConfig struct {
	Store                *Store
	HostFunctions        HostFunctions
	SystemMaxHTTPTimeout time.Duration
}

type Runtime struct {
	mu            sync.RWMutex
	runtime       wazero.Runtime
	store         *Store
	hostfns       HostFunctions
	registry      *PluginRegistry
	plugins       map[string]map[string]*Plugin
	pluginDirs    map[string]string
	triggerStops  map[string]context.CancelFunc
	schemaChanges map[string]SchemaChangeReport
	maxHTTP       time.Duration
}

func NewRuntime(ctx context.Context, cfg RuntimeConfig) *Runtime {
	maxHTTP := cfg.SystemMaxHTTPTimeout
	if maxHTTP <= 0 {
		maxHTTP = 60 * time.Second
	}
	return &Runtime{
		runtime:       wazero.NewRuntime(ctx),
		store:         cfg.Store,
		hostfns:       cfg.HostFunctions,
		registry:      NewPluginRegistry(),
		plugins:       make(map[string]map[string]*Plugin),
		pluginDirs:    make(map[string]string),
		triggerStops:  make(map[string]context.CancelFunc),
		schemaChanges: make(map[string]SchemaChangeReport),
		maxHTTP:       maxHTTP,
	}
}

func (r *Runtime) Close(ctx context.Context) error {
	if r == nil || r.runtime == nil {
		return nil
	}
	return r.runtime.Close(ctx)
}

func (r *Runtime) LoadAll(pluginsDir string, licence LicenceKey) error {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return fmt.Errorf("read plugins directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(pluginsDir, entry.Name())
		_, err := r.Load(dir, licence)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			slog.Warn("plugin load failed", "dir", dir, "error", err)
		}
	}
	return nil
}

func (r *Runtime) Load(pluginDir string, licence LicenceKey) (*Plugin, error) {
	p, err := r.loadPluginArtifact(pluginDir, licence)
	if err != nil {
		return nil, err
	}
	if err := r.putLoadedPlugin(p, pluginDir); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Runtime) loadPluginArtifact(pluginDir string, licence LicenceKey) (*Plugin, error) {
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	wasmPath := filepath.Join(pluginDir, "plugin.wasm")
	if _, err := os.Stat(manifestPath); err != nil {
		return nil, err
	}
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, err
	}
	manifest, warnings, manifestHash, err := ParseManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		slog.Warn("plugin manifest warning", "plugin_id", manifest.ID, "version", manifest.Version, "warning", w)
	}
	if manifest.Tier == "commercial" {
		allowed := licence != nil && licence.AllowsCommercialPlugin(manifest.ID)
		if !allowed {
			slog.Warn("plugin skipped because licence does not permit it", "plugin_id", manifest.ID, "version", manifest.Version)
			return nil, fmt.Errorf("plugin %s requires commercial licence", manifest.ID)
		}
	}

	module, err := r.runtime.CompileModule(context.Background(), wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile plugin wasm: %w", err)
	}
	wasmSum := sha256.Sum256(wasmBytes)
	wasmHash := hex.EncodeToString(wasmSum[:])
	p := &Plugin{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Version:      manifest.Version,
		Type:         PluginType(manifest.Type),
		Category:     manifest.Category,
		LicenceTier:  manifest.Tier,
		MaturityTier: manifest.Maturity,
		ToolCapable:  manifest.ToolCapable,
		ToolSafety:   manifest.ToolSafety,
		Manifest:     manifest,
		Module:       module,
		WASMHash:     wasmHash,
		ManifestHash: manifestHash,
		Status:       PluginActive,
	}
	return p, nil
}

func (r *Runtime) putLoadedPlugin(p *Plugin, pluginDir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.plugins[p.ID]
	if !ok {
		versions = make(map[string]*Plugin)
		r.plugins[p.ID] = versions
	}
	if _, exists := versions[p.Version]; exists {
		return fmt.Errorf("duplicate plugin: %s@%s", p.ID, p.Version)
	}
	versions[p.Version] = p
	if r.registry != nil {
		if err := r.registry.Register(p); err != nil {
			delete(versions, p.Version)
			return err
		}
	}
	r.pluginDirs[p.ID+"@"+p.Version] = pluginDir
	r.markLatestLocked(p.ID)
	return nil
}

func (r *Runtime) markLatestLocked(pluginID string) {
	versions := r.plugins[pluginID]
	list := make([]*Plugin, 0, len(versions))
	for _, p := range versions {
		p.IsLatest = false
		list = append(list, p)
	}
	sortPluginsByVersionDesc(list)
	if len(list) > 0 {
		list[0].IsLatest = true
	}
	if r.registry != nil {
		r.registry.UpdateLatest(pluginID)
	}
	ctx := context.Background()
	for _, p := range list {
		if r.store != nil {
			_ = r.store.UpsertPlugin(ctx, p)
		}
	}
	if r.store != nil && len(list) > 0 {
		_ = r.store.SetLatestByPluginID(ctx, pluginID, list[0].Version)
	}
}

func (r *Runtime) Unload(ref PluginRef) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.plugins[ref.ID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPluginNotLoaded, ref.ID)
	}
	if ref.Version == "" {
		delete(r.plugins, ref.ID)
		if r.registry != nil {
			_ = r.registry.Unregister(PluginRef{ID: ref.ID})
		}
		for key := range r.pluginDirs {
			if strings.HasPrefix(key, ref.ID+"@") {
				delete(r.pluginDirs, key)
			}
		}
		if r.store != nil {
			_ = r.store.DeleteByRef(context.Background(), ref)
		}
		return nil
	}
	if _, exists := versions[ref.Version]; !exists {
		return fmt.Errorf("%w: %s@%s", ErrPluginNotLoaded, ref.ID, ref.Version)
	}
	delete(versions, ref.Version)
	if r.registry != nil {
		_ = r.registry.Unregister(PluginRef{ID: ref.ID, Version: ref.Version})
	}
	delete(r.pluginDirs, ref.ID+"@"+ref.Version)
	if len(versions) == 0 {
		delete(r.plugins, ref.ID)
	} else {
		r.markLatestLocked(ref.ID)
	}
	if r.store != nil {
		_ = r.store.DeleteByRef(context.Background(), ref)
	}
	return nil
}

func (r *Runtime) Reload(ref PluginRef) error {
	current, err := r.Get(ref)
	if err != nil {
		return err
	}
	key := current.ID + "@" + current.Version

	r.mu.RLock()
	dir := r.pluginDirs[key]
	r.mu.RUnlock()
	if dir == "" {
		return fmt.Errorf("plugin source directory unknown for %s", key)
	}
	loaded, err := r.loadPluginArtifact(dir, AllowAllLicence{})
	if err != nil {
		return fmt.Errorf("reload %s failed; old module preserved: %w", key, err)
	}
	changes := DetectSchemaChanges(current.Manifest.UI.Properties, loaded.Manifest.UI.Properties)

	if err := r.Unload(PluginRef{ID: current.ID, Version: current.Version}); err != nil {
		return fmt.Errorf("unload %s before reload: %w", key, err)
	}
	if err := r.putLoadedPlugin(loaded, dir); err != nil {
		_ = r.putLoadedPlugin(current, dir)
		return fmt.Errorf("register %s after reload: %w", key, err)
	}

	if len(changes) > 0 {
		for _, change := range changes {
			slog.Warn("plugin schema change detected", "plugin_id", loaded.ID, "old_version", current.Version, "new_version", loaded.Version, "change_type", change.Type, "key", change.Key, "message", change.Message)
		}
		affected, impactErr := r.schemaImpact(loaded.ID, changes)
		if impactErr != nil {
			slog.Warn("plugin schema impact query failed", "plugin_id", loaded.ID, "error", impactErr)
		}
		r.storeSchemaChange(SchemaChangeReport{
			SchemaChange: SchemaChange{
				PluginID:   loaded.ID,
				OldVersion: current.Version,
				NewVersion: loaded.Version,
				Changes:    changes,
			},
			AffectedWorkflows: affected,
			RecordedAt:        time.Now().UTC(),
		})
	}
	return nil
}

func (r *Runtime) ExecuteStep(ctx context.Context, ref PluginRef, input StepInput) (StepResult, error) {
	p, err := r.Get(ref)
	if err != nil {
		return StepResult{Status: "error", Error: err.Error()}, err
	}
	if p.Status != PluginActive {
		return StepResult{Status: "error", Error: fmt.Sprintf("plugin %s@%s is %s", p.ID, p.Version, p.Status)}, fmt.Errorf("plugin %s@%s is %s", p.ID, p.Version, p.Status)
	}
	start := time.Now()
	if input.Timeout <= 0 {
		input.Timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, input.Timeout)
	defer cancel()

	instance, err := r.runtime.InstantiateModule(callCtx, p.Module, wazero.NewModuleConfig())
	if err != nil {
		r.recordInvocation(callCtx, p, input, nil, time.Since(start), "error", err, []byte("[]"))
		return StepResult{Status: "error", Error: err.Error()}, err
	}
	defer func() { _ = instance.Close(callCtx) }()

	result, execErr := executeModule(callCtx, instance, input.Data)
	status := "success"
	outRaw, _ := json.Marshal(result)
	if execErr != nil {
		status = "error"
	}
	r.recordInvocation(callCtx, p, input, outRaw, time.Since(start), status, execErr, []byte("[]"))
	if execErr != nil {
		return StepResult{Status: "error", Error: execErr.Error()}, execErr
	}
	return result, nil
}

func executeModule(ctx context.Context, mod api.Module, input []byte) (StepResult, error) {
	fn := mod.ExportedFunction("Execute")
	if fn == nil {
		fn = mod.ExportedFunction("call_get_age")
		if fn == nil {
			return StepResult{}, errors.New("plugin export Execute not found")
		}
		results, err := fn.Call(ctx)
		if err != nil {
			return StepResult{}, err
		}
		payload, _ := json.Marshal(map[string]any{"result": results})
		return StepResult{Status: "ok", Output: payload}, nil
	}

	def := mod.ExportedFunctionDefinitions()["Execute"]
	if def == nil {
		return StepResult{}, errors.New("plugin export Execute definition not found")
	}
	params := def.ParamTypes()
	if len(params) == 0 {
		results, err := fn.Call(ctx)
		if err != nil {
			return StepResult{}, err
		}
		payload, _ := json.Marshal(map[string]any{"result": results})
		return StepResult{Status: "ok", Output: payload}, nil
	}
	if len(params) != 2 {
		return StepResult{}, fmt.Errorf("unsupported Execute signature with %d params", len(params))
	}
	mem := mod.Memory()
	if mem == nil {
		return StepResult{}, errors.New("plugin module has no memory")
	}
	ptr, err := alloc(mod, uint32(len(input)))
	if err != nil {
		return StepResult{}, err
	}
	if ok := mem.Write(ptr, input); !ok {
		return StepResult{}, errors.New("failed writing input to wasm memory")
	}
	results, err := fn.Call(ctx, uint64(ptr), uint64(uint32(len(input))))
	if err != nil {
		return StepResult{}, err
	}
	if len(results) == 0 {
		return StepResult{Status: "ok"}, nil
	}
	packed := results[0]
	outPtr := uint32(packed >> 32)
	outLen := uint32(packed & 0xffffffff)
	raw, ok := mem.Read(outPtr, outLen)
	if !ok {
		return StepResult{}, errors.New("failed reading output from wasm memory")
	}
	result := StepResult{}
	if err := json.Unmarshal(raw, &result); err == nil && result.Status != "" {
		return result, nil
	}
	return StepResult{Status: "ok", Output: append([]byte(nil), raw...)}, nil
}

func alloc(mod api.Module, n uint32) (uint32, error) {
	malloc := mod.ExportedFunction("malloc")
	if malloc == nil {
		return 0, nil
	}
	results, err := malloc.Call(context.Background(), uint64(n))
	if err != nil {
		return 0, fmt.Errorf("malloc input buffer: %w", err)
	}
	if len(results) == 0 {
		return 0, errors.New("malloc returned no pointer")
	}
	return uint32(results[0]), nil
}

func (r *Runtime) recordInvocation(ctx context.Context, p *Plugin, input StepInput, output []byte, duration time.Duration, status string, runErr error, hostCalls []byte) {
	if r.store == nil || input.TenantID == uuid.Nil {
		return
	}
	inputHash := sha256.Sum256(input.Data)
	outHash := ""
	if len(output) > 0 {
		sum := sha256.Sum256(output)
		outHash = hex.EncodeToString(sum[:])
	}
	msg := ""
	if runErr != nil {
		msg = runErr.Error()
	}
	var caseID *uuid.UUID
	if input.CaseID != uuid.Nil {
		cid := input.CaseID
		caseID = &cid
	}
	inv := InvocationRecord{
		TenantID:       input.TenantID,
		PluginID:       p.ID,
		PluginVersion:  p.Version,
		WASMHash:       p.WASMHash,
		CaseID:         caseID,
		StepID:         input.StepID,
		InvocationType: "step_execute",
		InputHash:      hex.EncodeToString(inputHash[:]),
		OutputHash:     outHash,
		DurationMS:     int(duration.Milliseconds()),
		HostCalls:      hostCalls,
		Status:         invocationStatus(status, runErr),
		ErrorMessage:   msg,
	}
	_ = r.store.InsertInvocation(ctx, inv)
}

func invocationStatus(status string, runErr error) string {
	if status == "timeout" {
		return "timeout"
	}
	if runErr != nil {
		if errors.Is(runErr, context.DeadlineExceeded) {
			return "timeout"
		}
		return "error"
	}
	return "success"
}

func (r *Runtime) StartTrigger(ref PluginRef, config TriggerConfig) error {
	p, err := r.Get(ref)
	if err != nil {
		return err
	}
	if p.Type != TriggerPlugin {
		return fmt.Errorf("plugin %s@%s is not a trigger plugin", p.ID, p.Version)
	}
	ctx, cancel := context.WithCancel(context.Background())
	key := p.ID + "@" + p.Version

	r.mu.Lock()
	if existing, ok := r.triggerStops[key]; ok {
		existing()
	}
	r.triggerStops[key] = cancel
	r.mu.Unlock()

	go func() {
		instance, err := r.runtime.InstantiateModule(ctx, p.Module, wazero.NewModuleConfig())
		if err != nil {
			slog.Error("instantiate trigger plugin", "plugin", key, "error", err)
			return
		}
		defer func() { _ = instance.Close(context.Background()) }()
		startFn := instance.ExportedFunction("Start")
		if startFn == nil {
			slog.Warn("trigger plugin has no Start export", "plugin", key)
			return
		}
		_, _ = startFn.Call(ctx)
	}()
	return nil
}

func (r *Runtime) StopTrigger(ref PluginRef) error {
	p, err := r.Get(ref)
	if err != nil {
		return err
	}
	key := p.ID + "@" + p.Version

	r.mu.Lock()
	cancel, ok := r.triggerStops[key]
	if ok {
		delete(r.triggerStops, key)
	}
	r.mu.Unlock()
	if !ok {
		return nil
	}
	done := make(chan struct{})
	go func() {
		cancel()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("stop trigger timeout for %s", key)
	}
}

func (r *Runtime) List() []*Plugin {
	if r.registry == nil {
		return nil
	}
	return r.registry.All()
}

func (r *Runtime) Get(ref PluginRef) (*Plugin, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotLoaded, ref.ID)
	}
	return r.registry.ByRef(ref)
}

func (r *Runtime) ListVersions(pluginID string) ([]*Plugin, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotLoaded, pluginID)
	}
	return r.registry.ListVersions(pluginID)
}

func (r *Runtime) SetStatus(pluginID string, status PluginStatus) error {
	if r.registry == nil {
		return fmt.Errorf("%w: %s", ErrPluginNotLoaded, pluginID)
	}
	if err := r.registry.SetStatus(pluginID, status); err != nil {
		return err
	}
	versions, _ := r.registry.ListVersions(pluginID)
	for _, p := range versions {
		if r.store != nil {
			_ = r.store.UpsertPlugin(context.Background(), p)
		}
	}
	if r.store != nil {
		_ = r.store.SetStatusByPluginID(context.Background(), pluginID, status)
	}
	return nil
}

// RegisterVirtual registers a non-WASM plugin in the runtime registry so it
// appears in plugin APIs/palettes and can be managed like loaded plugins.
func (r *Runtime) RegisterVirtual(p *Plugin) error {
	if r == nil || p == nil {
		return fmt.Errorf("plugin is nil")
	}
	p.Module = nil
	p.WASMHash = ""
	p.ManifestHash = ""
	p.Status = PluginActive
	if p.Manifest.ID == "" {
		p.Manifest.ID = p.ID
	}
	if p.Manifest.Name == "" {
		p.Manifest.Name = p.Name
	}
	if p.Manifest.Version == "" {
		p.Manifest.Version = p.Version
	}
	if p.Manifest.Type == "" {
		p.Manifest.Type = string(p.Type)
	}
	if p.Manifest.Category == "" {
		p.Manifest.Category = p.Category
	}
	if p.Manifest.Tier == "" {
		p.Manifest.Tier = p.LicenceTier
	}
	if p.Manifest.Maturity == "" {
		p.Manifest.Maturity = p.MaturityTier
	}
	return r.putLoadedPlugin(p, "")
}

func clonePlugin(p *Plugin) *Plugin {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

func (r *Runtime) validateHostFunctionCall(plugin *Plugin, functionName string) error {
	if plugin == nil {
		return fmt.Errorf("plugin is nil")
	}
	for _, declared := range plugin.Manifest.HostFunctions {
		if declared == functionName {
			return nil
		}
	}
	err := fmt.Errorf("undeclared host function: %s", functionName)
	slog.Warn("blocked host function call", "plugin_id", plugin.ID, "plugin_version", plugin.Version, "function", functionName, "error", err)
	return err
}
