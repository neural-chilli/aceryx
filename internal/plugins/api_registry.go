package plugins

import (
	"context"
	"fmt"
	"strings"
)

func (r *Runtime) StepPalette() []PaletteCategory {
	if r == nil || r.registry == nil {
		return nil
	}
	return r.registry.StepPalette()
}

func (r *Runtime) ToolPalette() []PaletteCategory {
	if r == nil || r.registry == nil {
		return nil
	}
	return r.registry.ToolPalette()
}

func (r *Runtime) Search(query string) []*Plugin {
	if r == nil || r.registry == nil {
		return nil
	}
	return r.registry.Search(query)
}

func (r *Runtime) LastSchemaChange(pluginID string) (SchemaChangeReport, bool) {
	if r == nil {
		return SchemaChangeReport{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	report, ok := r.schemaChanges[strings.TrimSpace(pluginID)]
	if !ok {
		return SchemaChangeReport{}, false
	}
	return report, true
}

func (r *Runtime) storeSchemaChange(report SchemaChangeReport) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.schemaChanges == nil {
		r.schemaChanges = make(map[string]SchemaChangeReport)
	}
	r.schemaChanges[report.PluginID] = report
}

func (r *Runtime) schemaImpact(pluginID string, changes []PropertyChange) ([]WorkflowReference, error) {
	if r == nil || r.store == nil || r.store.db == nil {
		return nil, nil
	}
	refs, err := r.store.FindWorkflowsUsingPlugin(contextBackground(), pluginID, changes)
	if err != nil {
		return nil, fmt.Errorf("find workflows using changed schema: %w", err)
	}
	return refs, nil
}

func contextBackground() context.Context {
	return context.Background()
}
