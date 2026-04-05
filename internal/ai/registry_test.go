package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeTenantStore struct {
	byTenant map[uuid.UUID][]*AIComponentDef
}

func (f *fakeTenantStore) Create(_ context.Context, tenantID uuid.UUID, def *AIComponentDef, _ uuid.UUID) error {
	if f.byTenant == nil {
		f.byTenant = map[uuid.UUID][]*AIComponentDef{}
	}
	f.byTenant[tenantID] = append(f.byTenant[tenantID], cloneComponent(def))
	return nil
}
func (f *fakeTenantStore) Update(_ context.Context, tenantID uuid.UUID, def *AIComponentDef) error {
	items := f.byTenant[tenantID]
	for i := range items {
		if items[i].ID == def.ID {
			items[i] = cloneComponent(def)
			f.byTenant[tenantID] = items
			return nil
		}
	}
	return os.ErrNotExist
}
func (f *fakeTenantStore) Delete(_ context.Context, tenantID uuid.UUID, componentID string) error {
	items := f.byTenant[tenantID]
	out := items[:0]
	for _, item := range items {
		if item.ID != componentID {
			out = append(out, item)
		}
	}
	f.byTenant[tenantID] = out
	return nil
}
func (f *fakeTenantStore) ListByTenant(_ context.Context, tenantID uuid.UUID) ([]*AIComponentDef, error) {
	return f.byTenant[tenantID], nil
}

func TestRegistryLoadFromDirectorySkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		content := strings.ReplaceAll(validYAMLTemplate, "{id}", "comp_"+itoa(i))
		if err := os.WriteFile(filepath.Join(dir, "c"+itoa(i)+".yaml"), []byte(content), 0o644); err != nil {
			t.Fatalf("write yaml: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte("id: bad\noutput_schema: ["), 0o644); err != nil {
		t.Fatalf("write invalid yaml: %v", err)
	}

	r := NewComponentRegistry(nil)
	if err := r.LoadFromDirectory(dir); err != nil {
		t.Fatalf("load directory: %v", err)
	}
	items, err := r.List(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 valid components loaded, got %d", len(items))
	}
}

func TestRegistryTenantOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sentiment.yaml"), []byte(strings.ReplaceAll(validYAMLTemplate, "{id}", "sentiment_analysis")), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	store := &fakeTenantStore{byTenant: map[uuid.UUID][]*AIComponentDef{}}
	r := NewComponentRegistry(store)
	if err := r.LoadFromDirectory(dir); err != nil {
		t.Fatalf("load directory: %v", err)
	}
	tenantA := uuid.New()
	tenantB := uuid.New()
	override := mustDef(t, strings.ReplaceAll(validYAMLTemplate, "{id}", "sentiment_analysis"))
	override.DisplayLabel = "Custom Sentiment"
	if err := r.AddTenantComponent(context.Background(), tenantA, uuid.New(), override); err != nil {
		t.Fatalf("add tenant component: %v", err)
	}
	gotA, _ := r.Get(context.Background(), tenantA, "sentiment_analysis")
	if gotA.DisplayLabel != "Custom Sentiment" {
		t.Fatalf("expected tenant override for tenantA")
	}
	gotB, _ := r.Get(context.Background(), tenantB, "sentiment_analysis")
	if gotB.DisplayLabel == "Custom Sentiment" {
		t.Fatalf("tenantB should see global component")
	}
}

func TestRegistryListByCategory(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(strings.ReplaceAll(validYAMLTemplate, "{id}", "a")), 0o644)
	b := strings.ReplaceAll(validYAMLTemplate, "{id}", "b")
	b = strings.ReplaceAll(b, "AI: Text Analysis", "AI: Generation")
	_ = os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(b), 0o644)
	r := NewComponentRegistry(nil)
	if err := r.LoadFromDirectory(dir); err != nil {
		t.Fatalf("load directory: %v", err)
	}
	cats, err := r.ListByCategory(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("list by category: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}
}

func TestParseInitialYAMLComponents(t *testing.T) {
	base := filepath.Join("..", "..", "ai-components")
	files := []string{
		"sentiment_analysis.yaml",
		"document_classification.yaml",
		"pii_detection.yaml",
		"urgency_scoring.yaml",
		"keyword_extraction.yaml",
		"draft_response.yaml",
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(base, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := ParseComponentYAML(raw); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
	}
}

func TestParseComponentYAMLMissingRequiredField(t *testing.T) {
	_, err := ParseComponentYAML([]byte(`id: x\ndisplay_label: X\ncategory: Y\ninput_schema: {type: object}\noutput_schema: {type: object}\nuser_prompt_template: "{{.Input.text}}"`))
	if err == nil {
		t.Fatalf("expected parse failure")
	}
}

func mustDef(t *testing.T, raw string) *AIComponentDef {
	t.Helper()
	def, err := ParseComponentYAML([]byte(raw))
	if err != nil {
		t.Fatalf("parse component yaml: %v", err)
	}
	return def
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

const validYAMLTemplate = `
id: {id}
display_label: "Comp {id}"
category: "AI: Text Analysis"
description: "x"
icon: "x"
tier: commercial
input_schema:
  type: object
  properties:
    text:
      type: string
  required: [text]
output_schema:
  type: object
  properties:
    result:
      type: string
  required: [result]
system_prompt: "return json"
user_prompt_template: "{{.Input.text}}"
model_hints:
  preferred_size: small
  max_tokens: 200
`
