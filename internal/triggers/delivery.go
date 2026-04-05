package triggers

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/drivers"
)

type DeliveryHandler struct {
	mode          DeliveryMode
	driver        drivers.QueueDriver
	pipeline      ChannelPipeline
	tenantID      uuid.UUID
	onAsyncResult func(error)
}

func NewDeliveryHandler(mode DeliveryMode, driver drivers.QueueDriver, pipeline ChannelPipeline, tenantID uuid.UUID) *DeliveryHandler {
	return &DeliveryHandler{mode: mode, driver: driver, pipeline: pipeline, tenantID: tenantID}
}

func (dh *DeliveryHandler) ValidateStartup() error {
	if dh.mode != DeliveryExactlyOnce || dh.driver == nil {
		return nil
	}
	if cap, ok := dh.driver.(interface{ SupportsExactlyOnce() bool }); ok && cap.SupportsExactlyOnce() {
		return nil
	}
	id := strings.ToLower(strings.TrimSpace(dh.driver.ID()))
	if strings.Contains(id, "postgres") || strings.Contains(id, "pgmq") {
		return nil
	}
	return fmt.Errorf("exactly_once delivery not supported for driver: %s (use at_least_once)", dh.driver.ID())
}

func (dh *DeliveryHandler) HandleMessage(ctx context.Context, env messageEnvelope, channelID uuid.UUID) error {
	if dh.pipeline == nil {
		return fmt.Errorf("channel pipeline not configured")
	}
	if dh.driver == nil {
		return fmt.Errorf("queue driver not configured")
	}
	req := PipelineRequest{
		TenantID:  dh.tenantID,
		ChannelID: channelID,
		Data:      env.Message,
		Source:    "trigger",
	}

	switch dh.mode {
	case DeliveryBestEffort:
		if err := dh.driver.Ack(ctx, env.MessageID); err != nil {
			return err
		}
		go func() {
			_, err := dh.pipeline.Process(context.Background(), req)
			if dh.onAsyncResult != nil {
				dh.onAsyncResult(err)
			}
		}()
		return nil
	case DeliveryExactlyOnce, DeliveryAtLeastOnce:
		_, err := dh.pipeline.Process(ctx, req)
		if err != nil {
			_ = dh.driver.Nack(ctx, env.MessageID)
			return err
		}
		if err := dh.driver.Ack(ctx, env.MessageID); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported delivery mode: %s", dh.mode)
	}
}
