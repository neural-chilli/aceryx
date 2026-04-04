package plugins

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]map[string]*Plugin
	latest  map[string]*Plugin

	stepPaletteCache []PaletteCategory
	toolPaletteCache []PaletteCategory
	paletteDirty     bool
}

func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins:      make(map[string]map[string]*Plugin),
		latest:       make(map[string]*Plugin),
		paletteDirty: true,
	}
}

func (r *PluginRegistry) Register(p *Plugin) error {
	if p == nil {
		return fmt.Errorf("plugin is nil")
	}
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
	r.updateLatestLocked(p.ID)
	r.markDirtyLocked()
	return nil
}

func (r *PluginRegistry) Unregister(ref PluginRef) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.plugins[ref.ID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPluginNotLoaded, ref.ID)
	}
	if strings.TrimSpace(ref.Version) == "" {
		delete(r.plugins, ref.ID)
		delete(r.latest, ref.ID)
		r.markDirtyLocked()
		return nil
	}
	if _, ok := versions[ref.Version]; !ok {
		return fmt.Errorf("%w: %s@%s", ErrPluginNotLoaded, ref.ID, ref.Version)
	}
	delete(versions, ref.Version)
	if len(versions) == 0 {
		delete(r.plugins, ref.ID)
		delete(r.latest, ref.ID)
	} else {
		r.updateLatestLocked(ref.ID)
	}
	r.markDirtyLocked()
	return nil
}

func (r *PluginRegistry) UpdateLatest(pluginID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateLatestLocked(pluginID)
}

func (r *PluginRegistry) All() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Plugin, 0)
	for _, versions := range r.plugins {
		for _, p := range versions {
			out = append(out, clonePlugin(p))
		}
	}
	sortPlugins(out)
	return out
}

func (r *PluginRegistry) ByRef(ref PluginRef) (*Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, ok := r.plugins[ref.ID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotLoaded, ref.ID)
	}
	if ref.Version != "" {
		p, ok := versions[ref.Version]
		if !ok {
			return nil, fmt.Errorf("%w: %s@%s", ErrPluginNotLoaded, ref.ID, ref.Version)
		}
		return clonePlugin(p), nil
	}
	latest, ok := r.latest[ref.ID]
	if !ok || latest == nil {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotLoaded, ref.ID)
	}
	return clonePlugin(latest), nil
}

func (r *PluginRegistry) ByCategory(category string) []*Plugin {
	return r.filter(func(p *Plugin) bool {
		return strings.EqualFold(strings.TrimSpace(p.Category), strings.TrimSpace(category))
	})
}

func (r *PluginRegistry) ByType(pluginType PluginType) []*Plugin {
	return r.filter(func(p *Plugin) bool { return p.Type == pluginType })
}

func (r *PluginRegistry) ByMaturity(maturity string) []*Plugin {
	return r.filter(func(p *Plugin) bool {
		return strings.EqualFold(strings.TrimSpace(p.MaturityTier), strings.TrimSpace(maturity))
	})
}

func (r *PluginRegistry) ToolCapable() []*Plugin {
	return r.filter(func(p *Plugin) bool { return p.Manifest.ToolCapable })
}

func (r *PluginRegistry) Search(query string) []*Plugin {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return r.All()
	}
	return r.filter(func(p *Plugin) bool {
		return strings.Contains(strings.ToLower(p.Name), needle) ||
			strings.Contains(strings.ToLower(p.Manifest.UI.Description), needle) ||
			strings.Contains(strings.ToLower(p.Category), needle)
	})
}

func (r *PluginRegistry) StepPalette() []PaletteCategory {
	r.mu.RLock()
	if !r.paletteDirty && r.stepPaletteCache != nil {
		out := clonePaletteCategories(r.stepPaletteCache)
		r.mu.RUnlock()
		return out
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paletteDirty || r.stepPaletteCache == nil {
		all := r.allLocked()
		r.stepPaletteCache = buildPaletteCategories(all, false)
		r.toolPaletteCache = buildPaletteCategories(all, true)
		r.paletteDirty = false
	}
	return clonePaletteCategories(r.stepPaletteCache)
}

func (r *PluginRegistry) ToolPalette() []PaletteCategory {
	r.mu.RLock()
	if !r.paletteDirty && r.toolPaletteCache != nil {
		out := clonePaletteCategories(r.toolPaletteCache)
		r.mu.RUnlock()
		return out
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paletteDirty || r.toolPaletteCache == nil {
		all := r.allLocked()
		r.stepPaletteCache = buildPaletteCategories(all, false)
		r.toolPaletteCache = buildPaletteCategories(all, true)
		r.paletteDirty = false
	}
	return clonePaletteCategories(r.toolPaletteCache)
}

func (r *PluginRegistry) ListVersions(pluginID string) ([]*Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, ok := r.plugins[pluginID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotLoaded, pluginID)
	}
	out := make([]*Plugin, 0, len(versions))
	for _, p := range versions {
		out = append(out, clonePlugin(p))
	}
	sortPlugins(out)
	return out, nil
}

func (r *PluginRegistry) SetStatus(pluginID string, status PluginStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	versions, ok := r.plugins[pluginID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPluginNotLoaded, pluginID)
	}
	for _, p := range versions {
		p.Status = status
	}
	return nil
}

func (r *PluginRegistry) filter(match func(*Plugin) bool) []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Plugin, 0)
	for _, versions := range r.plugins {
		for _, p := range versions {
			if match(p) {
				out = append(out, clonePlugin(p))
			}
		}
	}
	sortPlugins(out)
	return out
}

func (r *PluginRegistry) allLocked() []*Plugin {
	out := make([]*Plugin, 0)
	for _, versions := range r.plugins {
		for _, p := range versions {
			out = append(out, clonePlugin(p))
		}
	}
	sortPlugins(out)
	return out
}

func (r *PluginRegistry) updateLatestLocked(pluginID string) {
	versions := r.plugins[pluginID]
	if len(versions) == 0 {
		delete(r.latest, pluginID)
		return
	}
	items := make([]*Plugin, 0, len(versions))
	for _, p := range versions {
		p.IsLatest = false
		items = append(items, p)
	}
	sort.Slice(items, func(i, j int) bool {
		cmp, err := compareSemver(items[i].Version, items[j].Version)
		if err != nil {
			return items[i].Version > items[j].Version
		}
		return cmp > 0
	})
	items[0].IsLatest = true
	r.latest[pluginID] = items[0]
}

func (r *PluginRegistry) markDirtyLocked() {
	r.paletteDirty = true
	r.stepPaletteCache = nil
	r.toolPaletteCache = nil
}

func sortPlugins(items []*Plugin) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		cmp, err := compareSemver(items[i].Version, items[j].Version)
		if err != nil {
			return items[i].Version > items[j].Version
		}
		return cmp > 0
	})
}

func clonePaletteCategories(in []PaletteCategory) []PaletteCategory {
	out := make([]PaletteCategory, 0, len(in))
	for _, category := range in {
		entryCopy := make([]PaletteEntry, 0, len(category.Plugins))
		for _, p := range category.Plugins {
			cp := p
			cp.Properties = append([]PropertyDef(nil), p.Properties...)
			entryCopy = append(entryCopy, cp)
		}
		out = append(out, PaletteCategory{Name: category.Name, Plugins: entryCopy})
	}
	return out
}
