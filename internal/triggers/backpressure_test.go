package triggers

import (
	"context"
	"testing"
	"time"
)

func TestBackpressureBlocksUntilDrain(t *testing.T) {
	inst := &TriggerInstance{buffer: make(chan messageEnvelope, 1)}
	inst.buffer <- messageEnvelope{MessageID: "first"}

	done := make(chan struct{})
	go func() {
		enqueueWithBackpressure(context.Background(), inst, messageEnvelope{MessageID: "second"}, 20*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("expected enqueue to block while full")
	case <-time.After(30 * time.Millisecond):
	}

	<-inst.buffer
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected enqueue to resume after drain")
	}
}
