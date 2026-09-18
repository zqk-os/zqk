package service

import (
	"context"
	"runtime"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/orchestration"
	"github.com/zqk-os/zqk/pkg/orchestration/ticker"
	"github.com/zqk-os/zqk/pkg/walutil"
)

// SynthesisService listens for intent events and triggers orchestration.
type SynthesisService struct {
	wal      *lifecycle.LifecycleEventWAL
	manager  orchestration.OrchestrationManager
	ticker   *ticker.ActivityTicker
	intentCh chan orchestration.RawIntent
}

// synthesisIntentWorkers caps concurrent ProcessIntent goroutines.
// TRACK: BLI-CAS-HAND-DUP-CHECK-001 — previously one goroutine per WAL intent.
func synthesisIntentWorkers() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
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

	goroutinelabels.NewGoroutine(orchestration.OpNameCapabilitySynthesis, "heartbeat").StartWithContext(ctx, func(gCtx context.Context) error {
		s.ticker.RunHeartbeat(gCtx, 30*time.Second)
		return nil
	})

	workers := synthesisIntentWorkers()
	s.intentCh = make(chan orchestration.RawIntent, workers*2)
	for i := 0; i < workers; i++ {
		goroutinelabels.NewGoroutine(orchestration.OpNameCapabilitySynthesis, "process-intent-worker").
			StartWithContext(ctx, func(gCtx context.Context) error {
				for {
					select {
					case <-gCtx.Done():
						return gCtx.Err()
					case intent := <-s.intentCh:
						if _, err := s.manager.ProcessIntent(gCtx, intent); err != nil {
							logging.GetLoggerFromContext(gCtx).LogError(orchestration.ErrMsgSynthesisError, err)
						}
					}
				}
			})
	}

	cursor := walutil.ReplayCursor{}
	poll := lifecycle.WALIdlePollMin

	for {
		var delivered int
		next, err := s.wal.ReplayFromCursor(cursor, func(ev *lifecycle.LifecycleEvent) error {
			delivered++
			if ev.EventType != orchestration.LogKeyIntentSubmission {
				return nil
			}

			s.ticker.Tick(orchestration.LogKeySynthesisAgent, orchestration.LogFmtOrchestratingIntent+ev.ID)

			intent := orchestration.RawIntent{
				Signature: ev.ID,
				Payload:   map[string]any{objects.FieldKeyScope: ev.Scope},
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case s.intentCh <- intent:
			}
			return nil
		})
		if err != nil {
			logging.FluentEvent(logger).Error(orchestration.ErrMsgSynthesisReplayFailed, err).Log()
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
