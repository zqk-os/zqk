package service

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/orchestration"
	"github.com/lanceman/zqk/pkg/orchestration/ticker"
)

// SynthesisService listens for intent events and triggers orchestration.
type SynthesisService struct {
	wal     *lifecycle.LifecycleEventWAL
	manager orchestration.OrchestrationManager
	ticker  *ticker.ActivityTicker
}

func NewSynthesisService(wal *lifecycle.LifecycleEventWAL, manager orchestration.OrchestrationManager, t *ticker.ActivityTicker) *SynthesisService {
	return &SynthesisService{
		wal:     wal,
		manager: manager,
		ticker:  t,
	}
}

// Run monitors the WAL and processes intents.
func (s *SynthesisService) Run(ctx context.Context) error {
	logger := logging.NewEventLogger(ctx)

	// Start heartbeat
	go s.ticker.RunHeartbeat(ctx, 30*time.Second)

	var lastSeq int64 = 0

	for {
		err := s.wal.ReplayFrom(lastSeq, func(ev *lifecycle.LifecycleEvent) error {
			if ev.EventType != orchestration.LogKeyIntentSubmission {
				return nil
			}

			// Update the ticker before starting orchestration
			s.ticker.Tick(orchestration.LogKeySynthesisAgent, orchestration.LogFmtOrchestratingIntent+ev.ID)

			// Trigger synthesis asynchronously using managed concurrency
			intent := orchestration.RawIntent{
				Signature: ev.ID,
				Payload:   map[string]any{objects.FieldKeyScope: ev.Scope},
			}
			builder := goroutinelabels.NewGoroutine(orchestration.OpNameCapabilitySynthesis, orchestration.LogMsgSynthesizingIntent+ev.ID)
			builder.StartWithContext(ctx, func(gCtx context.Context) error {
				if _, err := s.manager.ProcessIntent(gCtx, intent); err != nil {
					logging.GetLoggerFromContext(gCtx).LogError(orchestration.ErrMsgSynthesisError, err)
					return err
				}
				return nil
			})

			lastSeq = ev.Seq
			return nil
		})
		// ... (rest of the file)
		if err != nil {
			logging.FluentEvent(logger).Error(orchestration.ErrMsgSynthesisReplayFailed, err).Log()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Poll gracefully to avoid file handle exhaustion
		}
	}
}
