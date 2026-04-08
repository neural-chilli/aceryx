package assistant

import "testing"

func TestComputeUnifiedDiff(t *testing.T) {
	diff := computeUnifiedDiff("a: 1\nb: 2\n", "a: 1\nb: 3\n")
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
}

func TestExtractYAML(t *testing.T) {
	got := extractYAML("```yaml\nname: test\n```")
	if got != "name: test" {
		t.Fatalf("unexpected yaml extraction: %q", got)
	}
}
