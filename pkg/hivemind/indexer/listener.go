package indexer

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/walutil"
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

	cursor := walutil.ReplayCursor{}
	poll := lifecycle.WALIdlePollMin

	for {
		var delivered int
		next, err := l.wal.ReplayFromCursor(cursor, func(ev *lifecycle.LifecycleEvent) error {
			delivered++
			if ev.EventType != lifecycle.EventTypeStatusTransition {
				return nil
			}

			if ev.Kind != "requirement" && ev.Kind != "specification" {
				return nil
			}

			goroutinelabels.NewGoroutine(ConstIndexerListener, ConstIndexingObjectAsynchronously).StartSimple(func() {
				content := fmt.Sprintf(ConstObjectContentForS, ev.ID)

				ctx := context.Background()
				if err := l.worker.Index(ctx, ev.ID, content); err != nil {
					logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
					logging.Fluent(logger).Error(ConstErrorIndexingObjectS, err).ObjectID(ev.ID).Log()
				}
			})
			return nil
		})

		if err != nil {
			logging.FluentEvent(logger).Error(ConstIndexerListenerReplayFailed, err).Log()
		} else {
			cursor = next
		}

		if delivered > 0 {
			poll = lifecycle.WALIdlePollMin
		} else {
			poll = lifecycle.NextWALIdlePoll(poll)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}
