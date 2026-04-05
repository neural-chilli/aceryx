package ai

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type ComponentRegistry struct {
	mu        sync.RWMutex
	global    map[string]*AIComponentDef
	tenant    map[uuid.UUID]map[string]*AIComponentDef
	loaded    map[uuid.UUID]bool
	store     TenantComponentStore
	directory string
}

type ComponentCategory struct {
	Name       string            `json:"name"`
	Components []*AIComponentDef `json:"components"`
}

func NewComponentRegistry(store TenantComponentStore) *ComponentRegistry {
	return &ComponentRegistry{
		global: map[string]*AIComponentDef{},
		tenant: map[uuid.UUID]map[string]*AIComponentDef{},
		loaded: map[uuid.UUID]bool{},
		store:  store,
	}
}

func (r *ComponentRegistry) LoadFromDirectory(dir string) error {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return fmt.Errorf("component directory is required")
	}
	entries, err := os.ReadDir(trimmed)
	if err != nil {
		return fmt.Errorf("read component directory: %w", err)
	}
	loaded := map[string]*AIComponentDef{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isYAML(entry) {
			continue
		}
		fullPath := filepath.Join(trimmed, entry.Name())
		raw, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			slog.Warn("skip ai component file", "path", fullPath, "error", readErr)
			continue
		}
		def, parseErr := ParseComponentYAML(raw)
		if parseErr != nil {
			slog.Warn("skip invalid ai component file", "path", fullPath, "error", parseErr)
			continue
		}
		loaded[def.ID] = cloneComponent(def)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.global = loaded
	r.directory = trimmed
	return nil
}

func (r *ComponentRegistry) Reload() error {
	r.mu.RLock()
	dir := r.directory
	r.mu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("component directory not configured")
	}
	return r.LoadFromDirectory(dir)
}

func (r *ComponentRegistry) Get(ctx context.Context, tenantID uuid.UUID, componentID string) (*AIComponentDef, error) {
	if err := r.ensureTenantLoaded(ctx, tenantID); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(componentID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if def := r.tenant[tenantID][id]; def != nil {
		return cloneComponent(def), nil
	}
	if def := r.global[id]; def != nil {
		return cloneComponent(def), nil
	}
	return nil, fs.ErrNotExist
}

func (r *ComponentRegistry) List(ctx context.Context, tenantID uuid.UUID) ([]*AIComponentDef, error) {
	if err := r.ensureTenantLoaded(ctx, tenantID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	merged := map[string]*AIComponentDef{}
	for id, def := range r.global {
		merged[id] = cloneComponent(def)
	}
	for id, def := range r.tenant[tenantID] {
		merged[id] = cloneComponent(def)
	}
	ids := make([]string, 0, len(merged))
	for id := range merged {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*AIComponentDef, 0, len(ids))
	for _, id := range ids {
		out = append(out, merged[id])
	}
	return out, nil
}

func (r *ComponentRegistry) ListByCategory(ctx context.Context, tenantID uuid.UUID) ([]ComponentCategory, error) {
	all, err := r.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	byCategory := map[string][]*AIComponentDef{}
	for _, def := range all {
		cat := strings.TrimSpace(def.Category)
		if cat == "" {
			cat = "Uncategorised"
		}
		byCategory[cat] = append(byCategory[cat], def)
	}
	categoryNames := make([]string, 0, len(byCategory))
	for name := range byCategory {
		categoryNames = append(categoryNames, name)
	}
	sort.Strings(categoryNames)
	out := make([]ComponentCategory, 0, len(categoryNames))
	for _, name := range categoryNames {
		components := byCategory[name]
		sort.Slice(components, func(i, j int) bool {
			if components[i].DisplayLabel == components[j].DisplayLabel {
				return components[i].ID < components[j].ID
			}
			return components[i].DisplayLabel < components[j].DisplayLabel
		})
		out = append(out, ComponentCategory{Name: name, Components: components})
	}
	return out, nil
}

func (r *ComponentRegistry) AddTenantComponent(ctx context.Context, tenantID, createdBy uuid.UUID, def *AIComponentDef) error {
	if err := ValidateComponentDef(def); err != nil {
		return err
	}
	if r.store != nil {
		if err := r.store.Create(ctx, tenantID, def, createdBy); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tenant[tenantID] == nil {
		r.tenant[tenantID] = map[string]*AIComponentDef{}
	}
	r.tenant[tenantID][def.ID] = cloneComponent(def)
	r.loaded[tenantID] = true
	return nil
}

func (r *ComponentRegistry) UpdateTenantComponent(ctx context.Context, tenantID uuid.UUID, def *AIComponentDef) error {
	if err := ValidateComponentDef(def); err != nil {
		return err
	}
	if r.store != nil {
		if err := r.store.Update(ctx, tenantID, def); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tenant[tenantID] == nil {
		r.tenant[tenantID] = map[string]*AIComponentDef{}
	}
	r.tenant[tenantID][def.ID] = cloneComponent(def)
	r.loaded[tenantID] = true
	return nil
}

func (r *ComponentRegistry) DeleteTenantComponent(ctx context.Context, tenantID uuid.UUID, componentID string) error {
	trimmed := strings.TrimSpace(componentID)
	if r.store != nil {
		if err := r.store.Delete(ctx, tenantID, trimmed); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tenant[tenantID] != nil {
		delete(r.tenant[tenantID], trimmed)
	}
	r.loaded[tenantID] = true
	return nil
}

func (r *ComponentRegistry) ensureTenantLoaded(ctx context.Context, tenantID uuid.UUID) error {
	if r == nil {
		return fmt.Errorf("component registry is nil")
	}
	r.mu.RLock()
	alreadyLoaded := r.loaded[tenantID]
	r.mu.RUnlock()
	if alreadyLoaded || r.store == nil {
		return nil
	}
	defs, err := r.store.ListByTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("load tenant ai components: %w", err)
	}
	mapped := make(map[string]*AIComponentDef, len(defs))
	for _, def := range defs {
		if def == nil {
			continue
		}
		mapped[def.ID] = cloneComponent(def)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tenant[tenantID] = mapped
	r.loaded[tenantID] = true
	return nil
}

func isYAML(entry os.DirEntry) bool {
	name := strings.ToLower(entry.Name())
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func cloneComponent(in *AIComponentDef) *AIComponentDef {
	if in == nil {
		return nil
	}
	out := *in
	if len(in.InputSchema) > 0 {
		out.InputSchema = append([]byte(nil), in.InputSchema...)
	}
	if len(in.OutputSchema) > 0 {
		out.OutputSchema = append([]byte(nil), in.OutputSchema...)
	}
	if len(in.ConfigFields) > 0 {
		out.ConfigFields = make([]ConfigField, len(in.ConfigFields))
		copy(out.ConfigFields, in.ConfigFields)
	}
	if in.Confidence != nil {
		cfg := *in.Confidence
		out.Confidence = &cfg
	}
	return &out
}
