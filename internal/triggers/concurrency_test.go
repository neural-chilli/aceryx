package triggers

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConcurrencyParallelWorkerPool(t *testing.T) {
	drv := &mockQueueDriver{id: "nats"}
	var inFlight int64
	var maxInFlight int64
	h := NewDeliveryHandler(DeliveryAtLeastOnce, drv, &mockPipeline{fn: func(context.Context, PipelineRequest) (PipelineResult, error) {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			old := atomic.LoadInt64(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt64(&maxInFlight, old, cur) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		return PipelineResult{}, nil
	}}, uuid.New())

	inst := &TriggerInstance{}
	buf := make(chan messageEnvelope, 4)
	w1 := &worker{id: 1, channelID: uuid.New(), buffer: buf, handler: h, instance: inst}
	w2 := &worker{id: 2, channelID: uuid.New(), buffer: buf, handler: h, instance: inst}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); w1.run(ctx) }()
	go func() { defer wg.Done(); w2.run(ctx) }()

	for i := 0; i < 4; i++ {
		buf <- messageEnvelope{MessageID: "m"}
	}
	if !waitUntil(time.Second, func() bool { return atomic.LoadInt64(&inst.eventsProcessed) == 4 }) {
		t.Fatal("expected all messages processed")
	}
	if atomic.LoadInt64(&maxInFlight) < 2 {
		t.Fatalf("expected concurrent processing, max in flight=%d", maxInFlight)
	}
	cancel()
	close(buf)
	wg.Wait()
}
