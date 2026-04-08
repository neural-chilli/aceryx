package assistant

import (
	"os"
	"testing"
)

func TestBoolEnv(t *testing.T) {
	key := "ACERYX_TEST_BOOL_ENV"
	t.Setenv(key, "true")
	if !boolEnv(key) {
		t.Fatalf("expected true for 'true'")
	}
	t.Setenv(key, "1")
	if !boolEnv(key) {
		t.Fatalf("expected true for '1'")
	}
	t.Setenv(key, "yes")
	if !boolEnv(key) {
		t.Fatalf("expected true for 'yes'")
	}
	t.Setenv(key, "off")
	if boolEnv(key) {
		t.Fatalf("expected false for 'off'")
	}
	_ = os.Unsetenv(key)
	if boolEnv(key) {
		t.Fatalf("expected false when unset")
	}
}

func TestIntEnvOrDefault(t *testing.T) {
	key := "ACERYX_TEST_INT_ENV"
	t.Setenv(key, "500")
	if got := intEnvOrDefault(key, 42); got != 500 {
		t.Fatalf("expected 500, got %d", got)
	}
	t.Setenv(key, "0")
	if got := intEnvOrDefault(key, 42); got != 42 {
		t.Fatalf("expected fallback for 0, got %d", got)
	}
	t.Setenv(key, "not-a-number")
	if got := intEnvOrDefault(key, 42); got != 42 {
		t.Fatalf("expected fallback for invalid value, got %d", got)
	}
}

func TestTrimForLog(t *testing.T) {
	short := "abc"
	if got := trimForLog(short, 10); got != short {
		t.Fatalf("expected unmodified short string")
	}
	long := "abcdefghijklmnopqrstuvwxyz"
	got := trimForLog(long, 5)
	want := "abcde\n...[truncated]"
	if got != want {
		t.Fatalf("unexpected trimmed output: %q", got)
	}
}
