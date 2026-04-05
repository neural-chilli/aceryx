package rag

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Worker struct {
	pipeline *IngestionPipeline
	docStore DocumentStore
	interval time.Duration

	stopOnce sync.Once
	stopCh   chan struct{}
}

func NewWorker(pipeline *IngestionPipeline, docStore DocumentStore, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = time.Second
	}
	return &Worker{pipeline: pipeline, docStore: docStore, interval: interval, stopCh: make(chan struct{})}
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stopCh) })
}

func (w *Worker) tick(ctx context.Context) {
	doc, ok, err := w.docStore.ClaimPending(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "rag worker failed to claim pending document", "error", err)
		return
	}
	if !ok {
		return
	}
	if err := w.pipeline.ProcessDocument(ctx, doc.KnowledgeBaseID, doc.ID); err != nil {
		slog.ErrorContext(ctx, "rag worker failed to process document", "document_id", doc.ID.String(), "error", err)
	}
}
