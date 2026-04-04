package plugins

import "testing"

func TestDetectSchemaChanges(t *testing.T) {
	oldProps := []PropertyDef{
		{Key: "channel", Label: "Channel", Type: "text", HelpText: "Slack channel"},
		{Key: "message", Label: "Message", Type: "text"},
		{Key: "attempts", Label: "Attempts", Type: "number"},
	}
	newProps := []PropertyDef{
		{Key: "channel_id", Label: "Channel", Type: "text", HelpText: "Slack channel"},
		{Key: "message", Label: "Message", Type: "json"},
		{Key: "notify", Label: "Notify", Type: "boolean"},
	}
	changes := DetectSchemaChanges(oldProps, newProps)
	assertHasChangeType(t, changes, "renamed", "channel")
	assertHasChangeType(t, changes, "type_changed", "message")
	assertHasChangeType(t, changes, "removed", "attempts")
	assertHasChangeType(t, changes, "added", "notify")
}

func TestDetectSchemaChanges_NoChanges(t *testing.T) {
	props := []PropertyDef{
		{Key: "a", Type: "text"},
		{Key: "b", Type: "number"},
	}
	changes := DetectSchemaChanges(props, props)
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %#v", changes)
	}
}

func assertHasChangeType(t *testing.T, changes []PropertyChange, typ, key string) {
	t.Helper()
	for _, change := range changes {
		if change.Type == typ && change.Key == key {
			return
		}
	}
	t.Fatalf("expected change type=%q key=%q in %#v", typ, key, changes)
}
