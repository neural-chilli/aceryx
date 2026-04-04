package plugins

import "sort"

type PaletteCategory struct {
	Name    string         `json:"name"`
	Plugins []PaletteEntry `json:"plugins"`
}

type PaletteEntry struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	IconSVG      string        `json:"icon_svg"`
	Description  string        `json:"description"`
	CostLevel    string        `json:"cost_level"`
	MaturityTier string        `json:"maturity_tier"`
	ToolCapable  bool          `json:"tool_capable"`
	ToolSafety   string        `json:"tool_safety"`
	Properties   []PropertyDef `json:"properties"`
}

func buildPaletteCategories(plugins []*Plugin, toolOnly bool) []PaletteCategory {
	byCategory := make(map[string][]PaletteEntry)
	for _, p := range plugins {
		if p == nil {
			continue
		}
		if toolOnly && !p.Manifest.ToolCapable {
			continue
		}
		if !toolOnly && p.Type == TriggerPlugin {
			continue
		}
		category := p.Category
		if category == "" {
			category = "Uncategorized"
		}
		entry := PaletteEntry{
			ID:           p.ID,
			Name:         p.Name,
			Version:      p.Version,
			IconSVG:      p.Manifest.UI.IconSVG,
			Description:  p.Manifest.UI.Description,
			MaturityTier: p.MaturityTier,
			ToolCapable:  p.Manifest.ToolCapable,
			ToolSafety:   p.Manifest.ToolSafety,
			Properties:   append([]PropertyDef(nil), p.Manifest.UI.Properties...),
		}
		if p.Manifest.Cost != nil {
			entry.CostLevel = p.Manifest.Cost.Level
		}
		byCategory[category] = append(byCategory[category], entry)
	}

	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	out := make([]PaletteCategory, 0, len(categories))
	for _, category := range categories {
		entries := byCategory[category]
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Name != entries[j].Name {
				return entries[i].Name < entries[j].Name
			}
			cmp, err := compareSemver(entries[i].Version, entries[j].Version)
			if err != nil {
				return entries[i].Version > entries[j].Version
			}
			return cmp > 0
		})
		out = append(out, PaletteCategory{Name: category, Plugins: entries})
	}
	return out
}
