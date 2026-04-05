package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

func TestRedisDriverPublishConsume(t *testing.T) {
	mr := miniredis.RunT(t)
	d := New()
	if err := d.Connect(context.Background(), drivers.QueueConfig{Brokers: []string{mr.Addr()}}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = d.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, _, _, err := d.Consume(ctx, "events")
		errCh <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if err := d.Publish(context.Background(), "events", []byte("payload"), nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("consume: %v", err)
	}
	if err := d.Ack(context.Background(), "x"); err != nil {
		t.Fatalf("ack noop: %v", err)
	}
	if err := d.Nack(context.Background(), "x"); err != nil {
		t.Fatalf("nack noop: %v", err)
	}
}
