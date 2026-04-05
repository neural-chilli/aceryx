package local

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/neural-chilli/aceryx/internal/storage"
)

func TestStorePutGetListExistsDelete(t *testing.T) {
	ctx := context.Background()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	key := "tenant-a/2026/04/doc/file.txt"
	if err := store.Put(ctx, key, strings.NewReader("hello"), storage.ObjectMetadata{ContentType: "text/plain"}); err != nil {
		t.Fatalf("put: %v", err)
	}
	ok, err := store.Exists(ctx, key)
	if err != nil || !ok {
		t.Fatalf("exists: ok=%v err=%v", ok, err)
	}
	rc, meta, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = rc.Close() }()
	buf, _ := io.ReadAll(rc)
	if string(buf) != "hello" {
		t.Fatalf("unexpected body %q", string(buf))
	}
	if meta.ContentType != "text/plain" {
		t.Fatalf("expected content type text/plain, got %q", meta.ContentType)
	}
	items, err := store.List(ctx, "tenant-a/2026/04/", storage.ListOpts{MaxResults: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if _, err := store.PresignedURL(ctx, key, 15*time.Minute); err == nil {
		t.Fatal("expected presigned url to be unsupported for local store")
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ok, err = store.Exists(ctx, key)
	if err != nil || ok {
		t.Fatalf("exists after delete: ok=%v err=%v", ok, err)
	}
}
