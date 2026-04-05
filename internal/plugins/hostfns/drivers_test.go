package hostfns

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

type mockQueueDriver struct {
	consumeCalled bool
	ackCalled     bool
}

func (m *mockQueueDriver) ID() string                                             { return "mockq" }
func (m *mockQueueDriver) DisplayName() string                                    { return "Mock Queue" }
func (m *mockQueueDriver) Connect(_ context.Context, _ drivers.QueueConfig) error { return nil }
func (m *mockQueueDriver) Publish(_ context.Context, _ string, _ []byte, _ map[string]string) error {
	return nil
}
func (m *mockQueueDriver) Consume(_ context.Context, _ string) ([]byte, map[string]string, string, error) {
	m.consumeCalled = true
	return []byte("ok"), map[string]string{"k": "v"}, "mid", nil
}
func (m *mockQueueDriver) Ack(_ context.Context, _ string) error  { m.ackCalled = true; return nil }
func (m *mockQueueDriver) Nack(_ context.Context, _ string) error { return nil }
func (m *mockQueueDriver) Close() error                           { return nil }

type mockFileDriver struct{}

func (m *mockFileDriver) ID() string                                            { return "mockf" }
func (m *mockFileDriver) DisplayName() string                                   { return "Mock File" }
func (m *mockFileDriver) Connect(_ context.Context, _ drivers.FileConfig) error { return nil }
func (m *mockFileDriver) List(_ context.Context, _ string) ([]drivers.FileEntry, error) {
	return []drivers.FileEntry{{Path: "a.txt", Name: "a.txt", Size: 1, ModTime: time.Now()}}, nil
}
func (m *mockFileDriver) Read(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("x")), nil
}
func (m *mockFileDriver) Write(_ context.Context, _ string, _ io.Reader) error { return nil }
func (m *mockFileDriver) Delete(_ context.Context, _ string) error             { return nil }
func (m *mockFileDriver) Close() error                                         { return nil }

func TestQueueHostBridgeDelegates(t *testing.T) {
	reg := drivers.NewDriverRegistry()
	q := &mockQueueDriver{}
	reg.RegisterQueue(q)
	bridge := NewQueueBridge(reg)

	_, _, _, err := bridge.Consume("mockq", nil, "events")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if !q.consumeCalled {
		t.Fatal("expected consume to be called")
	}
	if err := bridge.Ack("mockq", "mid"); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if !q.ackCalled {
		t.Fatal("expected ack to be called")
	}
}

func TestFileWatchBridgeDelegates(t *testing.T) {
	reg := drivers.NewDriverRegistry()
	reg.RegisterFile(&mockFileDriver{})
	bridge := NewFileWatchBridge(reg)
	if _, err := bridge.Watch("mockf", nil, "."); err != nil {
		t.Fatalf("watch: %v", err)
	}
}
