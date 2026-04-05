package nats

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

type Driver struct {
	mu      sync.Mutex
	nc      *nats.Conn
	js      nats.JetStreamContext
	group   string
	pending map[string]*nats.Msg
}

func New() *Driver {
	return &Driver{pending: map[string]*nats.Msg{}}
}

func (d *Driver) ID() string          { return "nats" }
func (d *Driver) DisplayName() string { return "NATS JetStream" }

func (d *Driver) Connect(ctx context.Context, config drivers.QueueConfig) error {
	_ = ctx
	if len(config.Brokers) == 0 {
		return fmt.Errorf("nats broker is required")
	}
	opts := []nats.Option{}
	if config.Username != "" {
		opts = append(opts, nats.UserInfo(config.Username, config.Password))
	}
	nc, err := nats.Connect(strings.Join(config.Brokers, ","), opts...)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return fmt.Errorf("jetstream: %w", err)
	}
	group := config.ConsumerGroup
	if group == "" {
		group = "aceryx"
	}
	d.mu.Lock()
	d.nc = nc
	d.js = js
	d.group = group
	d.mu.Unlock()
	return nil
}

func (d *Driver) Publish(ctx context.Context, topic string, message []byte, headers map[string]string) error {
	d.mu.Lock()
	js := d.js
	d.mu.Unlock()
	if js == nil {
		return fmt.Errorf("nats not connected")
	}
	msg := &nats.Msg{Subject: topic, Data: message, Header: nats.Header{}}
	for k, v := range headers {
		msg.Header.Set(k, v)
	}
	_, err := js.PublishMsg(msg, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("publish nats: %w", err)
	}
	return nil
}

func (d *Driver) Consume(ctx context.Context, topic string) ([]byte, map[string]string, string, error) {
	d.mu.Lock()
	js := d.js
	group := d.group
	d.mu.Unlock()
	if js == nil {
		return nil, nil, "", fmt.Errorf("nats not connected")
	}
	sub, err := js.PullSubscribe(topic, group, nats.BindStream(streamName(topic)))
	if err != nil {
		// fallback when stream name is unknown
		sub, err = js.PullSubscribe(topic, group)
	}
	if err != nil {
		return nil, nil, "", fmt.Errorf("subscribe nats: %w", err)
	}
	msgs, err := sub.Fetch(1, nats.Context(ctx))
	if err != nil {
		return nil, nil, "", fmt.Errorf("fetch nats message: %w", err)
	}
	if len(msgs) == 0 {
		return nil, nil, "", fmt.Errorf("no nats message")
	}
	msg := msgs[0]
	metaMap := map[string]string{}
	for k := range msg.Header {
		metaMap[k] = msg.Header.Get(k)
	}
	mid := msg.Header.Get("Nats-Msg-Id")
	if mid == "" {
		if md, mdErr := msg.Metadata(); mdErr == nil {
			mid = fmt.Sprintf("%s:%d", md.Stream, md.Sequence.Stream)
			metaMap["stream"] = md.Stream
			metaMap["consumer"] = md.Consumer
		}
	}
	if mid == "" {
		mid = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	d.mu.Lock()
	d.pending[mid] = msg
	d.mu.Unlock()
	return append([]byte(nil), msg.Data...), metaMap, mid, nil
}

func (d *Driver) Ack(ctx context.Context, messageID string) error {
	_ = ctx
	msg := d.takePending(messageID)
	if msg == nil {
		return fmt.Errorf("message not found: %s", messageID)
	}
	if err := msg.Ack(); err != nil {
		return fmt.Errorf("ack nats: %w", err)
	}
	return nil
}

func (d *Driver) Nack(ctx context.Context, messageID string) error {
	_ = ctx
	msg := d.takePending(messageID)
	if msg == nil {
		return fmt.Errorf("message not found: %s", messageID)
	}
	if err := msg.Nak(); err != nil {
		return fmt.Errorf("nack nats: %w", err)
	}
	return nil
}

func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nc != nil {
		d.nc.Close()
		d.nc = nil
		d.js = nil
	}
	d.pending = map[string]*nats.Msg{}
	return nil
}

func (d *Driver) takePending(messageID string) *nats.Msg {
	d.mu.Lock()
	defer d.mu.Unlock()
	msg := d.pending[messageID]
	delete(d.pending, messageID)
	return msg
}

func streamName(topic string) string {
	clean := strings.ReplaceAll(topic, ".", "_")
	clean = strings.ReplaceAll(clean, "*", "all")
	clean = strings.ReplaceAll(clean, ">", "rest")
	return clean
}
