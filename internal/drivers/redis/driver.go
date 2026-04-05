package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/neural-chilli/aceryx/internal/drivers"
	goredis "github.com/redis/go-redis/v9"
)

type Driver struct {
	mu     sync.Mutex
	client *goredis.Client
	subs   map[string]*goredis.PubSub
}

func New() *Driver { return &Driver{subs: map[string]*goredis.PubSub{}} }

func (d *Driver) ID() string          { return "redis-pubsub" }
func (d *Driver) DisplayName() string { return "Redis Pub/Sub" }

func (d *Driver) Connect(ctx context.Context, config drivers.QueueConfig) error {
	if len(config.Brokers) == 0 {
		return fmt.Errorf("redis address is required")
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:     config.Brokers[0],
		Username: config.Username,
		Password: config.Password,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	d.mu.Lock()
	d.client = client
	d.mu.Unlock()
	return nil
}

func (d *Driver) Publish(ctx context.Context, topic string, message []byte, _ map[string]string) error {
	d.mu.Lock()
	client := d.client
	d.mu.Unlock()
	if client == nil {
		return fmt.Errorf("redis not connected")
	}
	if err := client.Publish(ctx, topic, message).Err(); err != nil {
		return fmt.Errorf("publish redis: %w", err)
	}
	return nil
}

func (d *Driver) Consume(ctx context.Context, topic string) ([]byte, map[string]string, string, error) {
	d.mu.Lock()
	client := d.client
	sub := d.subs[topic]
	if sub == nil && client != nil {
		sub = client.Subscribe(ctx, topic)
		d.subs[topic] = sub
	}
	d.mu.Unlock()
	if sub == nil {
		return nil, nil, "", fmt.Errorf("redis not connected")
	}
	msg, err := sub.ReceiveMessage(ctx)
	if err != nil {
		return nil, nil, "", fmt.Errorf("consume redis: %w", err)
	}
	messageID := fmt.Sprintf("%d", time.Now().UnixNano())
	meta := map[string]string{"channel": msg.Channel}
	return []byte(msg.Payload), meta, messageID, nil
}

func (d *Driver) Ack(ctx context.Context, messageID string) error {
	_ = ctx
	_ = messageID
	// Redis pub/sub is best-effort and has no ack semantic.
	return nil
}

func (d *Driver) Nack(ctx context.Context, messageID string) error {
	_ = ctx
	_ = messageID
	// Redis pub/sub is best-effort and has no nack semantic.
	return nil
}

func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for topic, sub := range d.subs {
		_ = sub.Close()
		delete(d.subs, topic)
	}
	if d.client != nil {
		return d.client.Close()
	}
	return nil
}
