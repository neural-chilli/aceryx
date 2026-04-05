package nats

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/test"
	natslib "github.com/nats-io/nats.go"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

func TestNATSDriverPublishConsumeAck(t *testing.T) {
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	s := natsserver.RunServer(&opts)
	defer s.Shutdown()

	nc, err := natslib.Connect(s.ClientURL())
	if err != nil {
		t.Fatalf("connect helper nats: %v", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream helper: %v", err)
	}
	_, _ = js.AddStream(&natslib.StreamConfig{Name: "events", Subjects: []string{"events"}})
	nc.Close()

	d := New()
	if err := d.Connect(context.Background(), drivers.QueueConfig{Brokers: []string{s.ClientURL()}, ConsumerGroup: "cg1"}); err != nil {
		t.Fatalf("connect driver: %v", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Publish(context.Background(), "events", []byte("hello"), map[string]string{"x-id": "1"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	msg, meta, messageID, err := d.Consume(ctx, "events")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if string(msg) != "hello" {
		t.Fatalf("unexpected message: %q", string(msg))
	}
	if messageID == "" {
		t.Fatal("expected messageID")
	}
	if len(meta) == 0 {
		t.Fatal("expected metadata")
	}
	if err := d.Ack(context.Background(), messageID); err != nil {
		t.Fatalf("ack: %v", err)
	}
}
