package hostfns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/neural-chilli/aceryx/internal/plugins"
	"gopkg.in/yaml.v3"
)

type QueueBridge struct {
	Registry *drivers.DriverRegistry

	mu        sync.Mutex
	connected map[string]drivers.QueueDriver
}

func NewQueueBridge(reg *drivers.DriverRegistry) *QueueBridge {
	return &QueueBridge{Registry: reg, connected: map[string]drivers.QueueDriver{}}
}

func (b *QueueBridge) Consume(driverID string, rawConfig []byte, topic string) ([]byte, map[string]string, string, error) {
	drv, err := b.ensureConnected(driverID, rawConfig)
	if err != nil {
		return nil, nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return drv.Consume(ctx, topic)
}

func (b *QueueBridge) Ack(driverID, messageID string) error {
	drv, err := b.Registry.GetQueue(driverID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return drv.Ack(ctx, messageID)
}

func (b *QueueBridge) ensureConnected(driverID string, rawConfig []byte) (drivers.QueueDriver, error) {
	if b.Registry == nil {
		return nil, fmt.Errorf("driver registry not configured")
	}
	drv, err := b.Registry.GetQueue(driverID)
	if err != nil {
		return nil, err
	}
	cfg := drivers.QueueConfig{}
	if len(rawConfig) > 0 {
		if err := yaml.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("decode queue config: %w", err)
		}
	}
	key := queueConnKey(driverID, cfg)
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.connected[key]; ok {
		return drv, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := drv.Connect(ctx, cfg); err != nil {
		return nil, err
	}
	b.connected[key] = drv
	return drv, nil
}

func queueConnKey(driverID string, cfg drivers.QueueConfig) string {
	raw := strings.Join(cfg.Brokers, ",") + "|" + cfg.Username + "|" + cfg.ConsumerGroup
	sum := sha256.Sum256([]byte(raw))
	return driverID + ":" + hex.EncodeToString(sum[:])
}

type FileWatchBridge struct {
	Registry *drivers.DriverRegistry

	mu        sync.Mutex
	connected map[string]drivers.FileDriver
	lastSeen  map[string]map[string]drivers.FileEntry
}

func NewFileWatchBridge(reg *drivers.DriverRegistry) *FileWatchBridge {
	return &FileWatchBridge{Registry: reg, connected: map[string]drivers.FileDriver{}, lastSeen: map[string]map[string]drivers.FileEntry{}}
}

func (b *FileWatchBridge) Watch(driverID string, rawConfig []byte, watchPath string) (plugins.FileEvent, error) {
	drv, watchKey, err := b.ensureConnected(driverID, rawConfig)
	if err != nil {
		return plugins.FileEvent{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	entries, err := drv.List(ctx, watchPath)
	if err != nil {
		return plugins.FileEvent{}, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	current := mapEntries(entries)
	prev := b.lastSeen[watchKey]
	b.lastSeen[watchKey] = current
	if len(prev) == 0 {
		return plugins.FileEvent{Path: watchPath, Operation: "snapshot", Metadata: map[string]string{"count": fmt.Sprintf("%d", len(entries))}}, nil
	}
	for _, ev := range diffEntries(prev, current) {
		return ev, nil
	}
	return plugins.FileEvent{Path: watchPath, Operation: "none"}, nil
}

func (b *FileWatchBridge) ensureConnected(driverID string, rawConfig []byte) (drivers.FileDriver, string, error) {
	if b.Registry == nil {
		return nil, "", fmt.Errorf("driver registry not configured")
	}
	drv, err := b.Registry.GetFile(driverID)
	if err != nil {
		return nil, "", err
	}
	cfg := drivers.FileConfig{}
	if len(rawConfig) > 0 {
		if err := yaml.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, "", fmt.Errorf("decode file config: %w", err)
		}
	}
	raw := cfg.Host + "|" + cfg.Username + "|" + cfg.BasePath
	sum := sha256.Sum256([]byte(raw))
	key := driverID + ":" + hex.EncodeToString(sum[:])

	b.mu.Lock()
	if _, ok := b.connected[key]; ok {
		b.mu.Unlock()
		return drv, key, nil
	}
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := drv.Connect(ctx, cfg); err != nil {
		return nil, "", err
	}

	b.mu.Lock()
	b.connected[key] = drv
	b.mu.Unlock()
	return drv, key, nil
}

func mapEntries(entries []drivers.FileEntry) map[string]drivers.FileEntry {
	m := make(map[string]drivers.FileEntry, len(entries))
	for _, e := range entries {
		m[e.Path] = e
	}
	return m
}

func diffEntries(prev, current map[string]drivers.FileEntry) []plugins.FileEvent {
	events := make([]plugins.FileEvent, 0)
	for path, cur := range current {
		old, ok := prev[path]
		if !ok {
			events = append(events, plugins.FileEvent{Path: path, Operation: "created", Metadata: map[string]string{"size": fmt.Sprintf("%d", cur.Size)}})
			continue
		}
		if old.Size != cur.Size || !old.ModTime.Equal(cur.ModTime) {
			events = append(events, plugins.FileEvent{Path: path, Operation: "modified", Metadata: map[string]string{"size": fmt.Sprintf("%d", cur.Size)}})
		}
	}
	for path := range prev {
		if _, ok := current[path]; !ok {
			events = append(events, plugins.FileEvent{Path: path, Operation: "deleted"})
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Operation == events[j].Operation {
			return events[i].Path < events[j].Path
		}
		return events[i].Operation < events[j].Operation
	})
	return events
}
