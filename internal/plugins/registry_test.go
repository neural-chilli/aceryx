package plugins

import (
	"fmt"
	"sync"
	"testing"
)

func TestPluginRegistry_RegisterUnregisterAndLatest(t *testing.T) {
	r := NewPluginRegistry()
	p1 := registryPlugin("slack", "1.0.0", StepPlugin, "Communication", false)
	p2 := registryPlugin("slack", "1.1.0", StepPlugin, "Communication", true)

	if err := r.Register(p1); err != nil {
		t.Fatalf("register p1: %v", err)
	}
	if err := r.Register(p2); err != nil {
		t.Fatalf("register p2: %v", err)
	}

	got, err := r.ByRef(PluginRef{ID: "slack"})
	if err != nil {
		t.Fatalf("ByRef latest: %v", err)
	}
	if got.Version != "1.1.0" || !got.IsLatest {
		t.Fatalf("expected latest 1.1.0, got %#v", got)
	}

	if err := r.Unregister(PluginRef{ID: "slack", Version: "1.1.0"}); err != nil {
		t.Fatalf("unregister latest: %v", err)
	}
	got, err = r.ByRef(PluginRef{ID: "slack"})
	if err != nil {
		t.Fatalf("ByRef after unregister: %v", err)
	}
	if got.Version != "1.0.0" || !got.IsLatest {
		t.Fatalf("expected fallback latest 1.0.0, got %#v", got)
	}
}

func TestPluginRegistry_DuplicateRejected(t *testing.T) {
	r := NewPluginRegistry()
	p := registryPlugin("slack", "1.0.0", StepPlugin, "Communication", false)
	if err := r.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(p); err == nil || err.Error() != "duplicate plugin: slack@1.0.0" {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestPluginRegistry_FilterSearch(t *testing.T) {
	r := NewPluginRegistry()
	companyPlugin := registryPlugin("companies-house", "1.0.0", StepPlugin, "Financial Services", true)
	companyPlugin.Name = "Companies House"
	companyPlugin.Manifest.UI.Description = "Company search"
	_ = r.Register(companyPlugin)
	_ = r.Register(registryPlugin("open-banking", "1.0.0", TriggerPlugin, "Financial Services", false))
	_ = r.Register(registryPlugin("slack", "1.0.0", StepPlugin, "Communication", false))

	if got := len(r.ByCategory("Financial Services")); got != 2 {
		t.Fatalf("ByCategory expected 2, got %d", got)
	}
	if got := len(r.ByType(StepPlugin)); got != 2 {
		t.Fatalf("ByType step expected 2, got %d", got)
	}
	if got := len(r.ByMaturity("community")); got != 3 {
		t.Fatalf("ByMaturity expected 3, got %d", got)
	}
	if got := len(r.ToolCapable()); got != 1 {
		t.Fatalf("ToolCapable expected 1, got %d", got)
	}
	results := r.Search("company")
	if len(results) != 1 || results[0].ID != "companies-house" {
		t.Fatalf("Search(company) expected companies-house, got %#v", results)
	}
}

func TestPluginRegistry_PaletteGenerationAndInvalidation(t *testing.T) {
	r := NewPluginRegistry()
	_ = r.Register(registryPlugin("companies-house", "1.0.0", StepPlugin, "Financial Services", true))
	_ = r.Register(registryPlugin("salesforce-sync", "1.0.0", StepPlugin, "CRM", false))
	_ = r.Register(registryPlugin("queue-trigger", "1.0.0", TriggerPlugin, "Messaging", true))

	steps := r.StepPalette()
	if len(steps) != 2 {
		t.Fatalf("step palette expected 2 categories (excluding trigger), got %d", len(steps))
	}
	tools := r.ToolPalette()
	if len(tools) != 2 {
		t.Fatalf("tool palette expected 2 categories (tool-capable only), got %d", len(tools))
	}

	_ = r.Unregister(PluginRef{ID: "salesforce-sync", Version: "1.0.0"})
	stepsAfter := r.StepPalette()
	if len(stepsAfter) != 1 {
		t.Fatalf("step palette should be invalidated after unregister, got %d categories", len(stepsAfter))
	}
}

func TestPluginRegistry_ConcurrentAccess(t *testing.T) {
	r := NewPluginRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = r.Register(registryPlugin(fmt.Sprintf("plugin-%d", i), "1.0.0", StepPlugin, "General", i%2 == 0))
		}(i)
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.All()
			_ = r.StepPalette()
			_ = r.ToolPalette()
		}()
	}
	wg.Wait()

	if got := len(r.All()); got != 25 {
		t.Fatalf("expected 25 plugins after concurrent register, got %d", got)
	}
}

func registryPlugin(id, version string, typ PluginType, category string, toolCapable bool) *Plugin {
	return &Plugin{
		ID:           id,
		Name:         id,
		Version:      version,
		Type:         typ,
		Category:     category,
		MaturityTier: "community",
		Status:       PluginActive,
		Manifest: PluginManifest{
			ID:          id,
			Name:        id,
			Version:     version,
			Type:        string(typ),
			Category:    category,
			Tier:        "open_source",
			Maturity:    "community",
			ToolCapable: toolCapable,
			UI: ManifestUI{
				Description: id + " description",
			},
			Cost: &CostMeta{Level: "low"},
		},
	}
}
