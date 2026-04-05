package localfs

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

func TestLocalFSListReadWriteDeleteAndTraversalGuard(t *testing.T) {
	d := New()
	base := t.TempDir()
	if err := d.Connect(context.Background(), drivers.FileConfig{BasePath: base}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := d.Write(context.Background(), "in/a.txt", bytes.NewBufferString("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, err := d.List(context.Background(), "in")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	rc, err := d.Read(context.Background(), "in/a.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(rc)
	if string(b) != "hello" {
		t.Fatalf("unexpected file contents: %q", string(b))
	}
	if err := d.Delete(context.Background(), "in/a.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := d.Read(context.Background(), "../../etc/passwd"); err == nil {
		t.Fatal("expected traversal protection error")
	}
}
