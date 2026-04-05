package drivers

import (
	"context"
	"database/sql"
	"testing"
)

type fakeDBDriver struct{ id, name string }

func (f fakeDBDriver) ID() string                                             { return f.id }
func (f fakeDBDriver) DisplayName() string                                    { return f.name }
func (f fakeDBDriver) Connect(_ context.Context, _ DBConfig) (*sql.DB, error) { return nil, nil }
func (f fakeDBDriver) Ping(_ context.Context, _ *sql.DB) error                { return nil }
func (f fakeDBDriver) Close(_ *sql.DB) error                                  { return nil }

type fakeQueueDriver struct{ id, name string }

func (f fakeQueueDriver) ID() string                                     { return f.id }
func (f fakeQueueDriver) DisplayName() string                            { return f.name }
func (f fakeQueueDriver) Connect(_ context.Context, _ QueueConfig) error { return nil }
func (f fakeQueueDriver) Publish(_ context.Context, _ string, _ []byte, _ map[string]string) error {
	return nil
}
func (f fakeQueueDriver) Consume(_ context.Context, _ string) ([]byte, map[string]string, string, error) {
	return nil, nil, "", nil
}
func (f fakeQueueDriver) Ack(_ context.Context, _ string) error  { return nil }
func (f fakeQueueDriver) Nack(_ context.Context, _ string) error { return nil }
func (f fakeQueueDriver) Close() error                           { return nil }

func TestDriverRegistryRegisterLookupList(t *testing.T) {
	r := NewDriverRegistry()
	r.RegisterDB(fakeDBDriver{id: "postgres", name: "PostgreSQL"})
	r.RegisterQueue(fakeQueueDriver{id: "nats", name: "NATS"})

	if _, err := r.GetDB("postgres"); err != nil {
		t.Fatalf("GetDB: %v", err)
	}
	if _, err := r.GetQueue("nats"); err != nil {
		t.Fatalf("GetQueue: %v", err)
	}
	if _, err := r.GetDB("missing"); err == nil {
		t.Fatal("expected missing db error")
	}
	if got := len(r.AllDB()); got != 1 {
		t.Fatalf("expected 1 db driver, got %d", got)
	}
	if got := len(r.AllQueues()); got != 1 {
		t.Fatalf("expected 1 queue driver, got %d", got)
	}
}
