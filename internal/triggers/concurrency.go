package triggers

import (
	"context"

	"github.com/google/uuid"
)

type worker struct {
	id        int
	channelID uuid.UUID
	buffer    <-chan messageEnvelope
	handler   *DeliveryHandler
	instance  *TriggerInstance
}

func (w *worker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case env, ok := <-w.buffer:
			if !ok {
				return
			}
			w.instance.recordReceived()
			err := w.handler.HandleMessage(ctx, env, w.channelID)
			if err != nil {
				w.instance.recordFailed()
			} else {
				w.instance.recordProcessed()
			}
		}
	}
}
