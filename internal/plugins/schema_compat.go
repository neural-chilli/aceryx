package plugins

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SchemaChange struct {
	PluginID   string           `json:"plugin_id"`
	OldVersion string           `json:"old_version"`
	NewVersion string           `json:"new_version"`
	Changes    []PropertyChange `json:"changes"`
}

type PropertyChange struct {
	Type    string `json:"type"`
	Key     string `json:"key"`
	OldType string `json:"old_type,omitempty"`
	NewType string `json:"new_type,omitempty"`
	Message string `json:"message"`
}

type SchemaChangeReport struct {
	SchemaChange
	AffectedWorkflows []WorkflowReference `json:"affected_workflows,omitempty"`
	RecordedAt        time.Time           `json:"recorded_at"`
}

type WorkflowReference struct {
	WorkflowID   uuid.UUID `json:"workflow_id"`
	WorkflowName string    `json:"workflow_name"`
	Version      int       `json:"version"`
}

func DetectSchemaChanges(oldProps, newProps []PropertyDef) []PropertyChange {
	oldByKey := make(map[string]PropertyDef, len(oldProps))
	newByKey := make(map[string]PropertyDef, len(newProps))
	for _, item := range oldProps {
		oldByKey[item.Key] = item
	}
	for _, item := range newProps {
		newByKey[item.Key] = item
	}

	out := make([]PropertyChange, 0)
	removed := make([]PropertyDef, 0)
	added := make([]PropertyDef, 0)

	for key, old := range oldByKey {
		if updated, ok := newByKey[key]; ok {
			if old.Type != updated.Type {
				out = append(out, PropertyChange{
					Type:    "type_changed",
					Key:     key,
					OldType: old.Type,
					NewType: updated.Type,
					Message: fmt.Sprintf("property '%s' changed type from '%s' to '%s'", key, old.Type, updated.Type),
				})
			}
			continue
		}
		removed = append(removed, old)
	}
	for key, item := range newByKey {
		if _, ok := oldByKey[key]; !ok {
			added = append(added, item)
		}
	}

	usedAdded := make(map[string]struct{})
	for _, old := range removed {
		match := findRenameCandidate(old, added, usedAdded)
		if match == nil {
			continue
		}
		usedAdded[match.Key] = struct{}{}
		out = append(out, PropertyChange{
			Type:    "renamed",
			Key:     old.Key,
			OldType: old.Type,
			NewType: match.Type,
			Message: fmt.Sprintf("property '%s' renamed to '%s'", old.Key, match.Key),
		})
	}

	for _, old := range removed {
		if wasRenamed(old, out) {
			continue
		}
		out = append(out, PropertyChange{
			Type:    "removed",
			Key:     old.Key,
			OldType: old.Type,
			Message: fmt.Sprintf("property '%s' was removed", old.Key),
		})
	}
	for _, item := range added {
		if _, ok := usedAdded[item.Key]; ok {
			continue
		}
		out = append(out, PropertyChange{
			Type:    "added",
			Key:     item.Key,
			NewType: item.Type,
			Message: fmt.Sprintf("property '%s' was added", item.Key),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func findRenameCandidate(old PropertyDef, added []PropertyDef, used map[string]struct{}) *PropertyDef {
	var candidate *PropertyDef
	for i := range added {
		item := added[i]
		if _, already := used[item.Key]; already {
			continue
		}
		if !strings.EqualFold(old.Type, item.Type) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(old.Label), strings.TrimSpace(item.Label)) ||
			strings.EqualFold(strings.TrimSpace(old.HelpText), strings.TrimSpace(item.HelpText)) {
			candidate = &added[i]
			break
		}
	}
	return candidate
}

func wasRenamed(old PropertyDef, changes []PropertyChange) bool {
	for _, change := range changes {
		if change.Type == "renamed" && change.Key == old.Key {
			return true
		}
	}
	return false
}
