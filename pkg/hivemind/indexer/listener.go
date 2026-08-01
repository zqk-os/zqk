package indexer

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"

	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/logging"
)

// IndexerListener watches lifecycle events and triggers indexing.
type IndexerListener struct {
	wal    *lifecycle.LifecycleEventWAL
	worker *IndexerWorker
}

func NewIndexerListener(wal *lifecycle.LifecycleEventWAL, worker *IndexerWorker) *IndexerListener {
	return &IndexerListener{
		wal:    wal,
		worker: worker,
	}
}

// Run starts the listener in a background goroutine.
func (l *IndexerListener) Run(ctx context.Context) error {
	logger := logging.NewEventLogger(ctx)

	// This channel receives status transitions
	// The lifecycle WAL replay is the primary source of truth.
	// For simplicity in this implementation, we will replay from the beginning
	// and then wait for new events.

	// Assuming a mechanism to get the last processed sequence
	var lastSeq int64 = 0

	for {
		err := l.wal.ReplayFrom(lastSeq, func(ev *lifecycle.LifecycleEvent) error {
			if ev.EventType != lifecycle.EventTypeStatusTransition {
				return nil
			}

			if ev.Kind != "requirement" && ev.Kind != "specification" {
				return nil
			}

			// Trigger indexing asynchronously
			goroutinelabels.NewGoroutine(ConstIndexerListener, ConstIndexingObjectAsynchronously).StartSimple(func() {
				// We need to fetch the content.
				// In a real ZQK system, this would use a storage provider or object loader.
				// Placeholder for now.
				content := fmt.Sprintf(ConstObjectContentForS, ev.ID)

				ctx := context.Background() // Should use a proper lifecycle context
				if err := l.worker.Index(ctx, ev.ID, content); err != nil {
					// Handle error
					logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
					logging.Fluent(logger).Error(ConstErrorIndexingObjectS, err).String("id", ev.ID).Log()
				}
			})

			lastSeq = ev.Seq
			return nil
		})

		if err != nil {
			logging.FluentEvent(logger).Error(ConstIndexerListenerReplayFailed, err).Log()
		}

		// Simple polling interval or waiting for signal
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			// Poll
		}
	}
}
