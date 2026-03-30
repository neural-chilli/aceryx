package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContentHashDeterministic(t *testing.T) {
	input := []byte("hello vault")
	h1 := ContentHash(input)
	h2 := ContentHash(input)
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %s and %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("expected sha256 hash length 64, got %d", len(h1))
	}
}

func TestDisplayModeForMime(t *testing.T) {
	cases := map[string]string{
		"application/pdf": "inline",
		"image/png":       "inline",
		"image/jpeg":      "inline",
		"image/gif":       "inline",
		"image/webp":      "inline",
		"text/plain":      "inline",
		"text/markdown":   "inline",
		"text/csv":        "inline",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "download",
	}
	for mime, expected := range cases {
		if got := DisplayModeForMime(mime); got != expected {
			t.Fatalf("mime %s expected %s got %s", mime, expected, got)
		}
	}
}

func TestSignedURLVerify(t *testing.T) {
	store := NewLocalVaultStore(t.TempDir(), "secret")
	store.now = func() time.Time { return time.Unix(1000, 0).UTC() }
	signed, err := store.SignedURL("tenant-a/2026/03/aa/bb/hash.pdf", 60)
	if err != nil {
		t.Fatalf("signed url: %v", err)
	}
	parts := strings.SplitN(signed, "?", 2)
	if len(parts) != 2 {
		t.Fatalf("expected query in signed url: %s", signed)
	}
	q := map[string]string{}
	for _, kv := range strings.Split(parts[1], "&") {
		p := strings.SplitN(kv, "=", 2)
		if len(p) == 2 {
			q[p[0]] = p[1]
		}
	}
	if err := store.VerifySignedURL(parts[0], q["exp"], q["sig"]); err != nil {
		t.Fatalf("verify signed url: %v", err)
	}

	if err := store.VerifySignedURL(parts[0], q["exp"], "deadbeef"); err == nil {
		t.Fatal("expected tampered signature rejection")
	}

	store.now = func() time.Time { return time.Unix(5000, 0).UTC() }
	if err := store.VerifySignedURL(parts[0], q["exp"], q["sig"]); err == nil {
		t.Fatal("expected expired signed url rejection")
	}
}

func TestLocalVaultStorePutGetDelete(t *testing.T) {
	root := t.TempDir()
	store := NewLocalVaultStore(root, "secret")
	store.now = func() time.Time { return time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC) }

	hash := strings.Repeat("ab", 32)
	uri, err := store.Put("tenant-a", hash, "pdf", []byte("payload"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	expectedPath := filepath.Join(root, "tenant-a", "2026", "03", "ab", "ab", hash+".pdf")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected file at %s: %v", expectedPath, err)
	}
	if uri != filepath.ToSlash(strings.TrimPrefix(expectedPath, root+string(filepath.Separator))) {
		t.Fatalf("unexpected uri %s", uri)
	}

	data, err := store.Get(uri)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected data %q", string(data))
	}

	if err := store.Delete(uri); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Fatalf("expected deleted file, got err=%v", err)
	}
}
