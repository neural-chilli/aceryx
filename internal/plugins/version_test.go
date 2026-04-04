package plugins

import "testing"

func TestCompareSemver(t *testing.T) {
	cmp, err := compareSemver("1.2.0", "1.1.9")
	if err != nil {
		t.Fatalf("compareSemver error: %v", err)
	}
	if cmp <= 0 {
		t.Fatalf("expected 1.2.0 > 1.1.9, got %d", cmp)
	}
	_, err = compareSemver("bad", "1.0.0")
	if err == nil {
		t.Fatal("expected error for invalid semver")
	}
}
