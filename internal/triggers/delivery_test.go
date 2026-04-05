package triggers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeliveryAtLeastOnceAckAndNack(t *testing.T) {
	tenantID := uuid.New()
	drv := &mockQueueDriver{id: "nats"}
	h := NewDeliveryHandler(DeliveryAtLeastOnce, drv, &mockPipeline{}, tenantID)
	env := messageEnvelope{Message: []byte(`{"x":1}`), MessageID: "m1"}
	if err := h.HandleMessage(context.Background(), env, uuid.New()); err != nil {
		t.Fatalf("handle success: %v", err)
	}
	if drv.ack != 1 || drv.nack != 0 {
		t.Fatalf("expected ack=1 nack=0 got ack=%d nack=%d", drv.ack, drv.nack)
	}

	h.pipeline = &mockPipeline{fn: func(context.Context, PipelineRequest) (PipelineResult, error) {
		return PipelineResult{}, errors.New("boom")
	}}
	if err := h.HandleMessage(context.Background(), env, uuid.New()); err == nil {
		t.Fatal("expected error")
	}
	if drv.nack != 1 {
		t.Fatalf("expected nack=1, got %d", drv.nack)
	}
}

func TestDeliveryBestEffortAcksImmediately(t *testing.T) {
	tenantID := uuid.New()
	drv := &mockQueueDriver{id: "nats"}
	pipelineDone := make(chan struct{})
	h := NewDeliveryHandler(DeliveryBestEffort, drv, &mockPipeline{fn: func(context.Context, PipelineRequest) (PipelineResult, error) {
		close(pipelineDone)
		return PipelineResult{}, errors.New("lost")
	}}, tenantID)
	env := messageEnvelope{Message: []byte(`{"x":1}`), MessageID: "m1"}
	if err := h.HandleMessage(context.Background(), env, uuid.New()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if drv.ack != 1 {
		t.Fatalf("expected immediate ack")
	}
	select {
	case <-pipelineDone:
	case <-time.After(time.Second):
		t.Fatal("expected async pipeline execution")
	}
}

func TestDeliveryExactlyOnceValidation(t *testing.T) {
	h := NewDeliveryHandler(DeliveryExactlyOnce, &mockQueueDriver{id: "kafka"}, &mockPipeline{}, uuid.New())
	if err := h.ValidateStartup(); err == nil {
		t.Fatal("expected exactly_once rejection for kafka")
	}

	h = NewDeliveryHandler(DeliveryExactlyOnce, &mockExactlyOnceDriver{mockQueueDriver{id: "custom"}}, &mockPipeline{}, uuid.New())
	if err := h.ValidateStartup(); err != nil {
		t.Fatalf("expected exactly_once support: %v", err)
	}
}
