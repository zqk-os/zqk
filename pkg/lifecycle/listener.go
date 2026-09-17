// Package lifecycle: listener reads the lifecycle event WAL, accumulates satisfied criteria,
// and when all criteria for a transition rule are met, sends the transition to the updater channel.

package lifecycle

import (
	"context"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/walutil"
)

// TransitionRequest is a request to apply a status transition (kind, id, to_status).
type TransitionRequest struct {
	Kind     string
	ID       string
	ToStatus string
}

// Listener reads the lifecycle WAL, accumulates criteria, and enqueues transition requests.
// Run one listener per project root (single goroutine). Bounded: no unbounded goroutines.
type Listener struct {
	projectRoot  string
	wal          *LifecycleEventWAL
	rules        []TransitionRule
	transitionCh chan<- TransitionRequest
	getStorage   StorageProviderForCriterion

	// satisfied: set of (criterion_id, scope_key) that have been satisfied
	satisfied      map[string]struct{}
	fired          map[string]struct{} // rule key -> fired, so we don't double-fire
	cursor         walutil.ReplayCursor
	mu             sync.Mutex
	checkpointPath string
}

// NewListener creates a listener that reads from wal, evaluates rules, and sends transitions to transitionCh.
func NewListener(projectRoot string, wal *LifecycleEventWAL, rules []TransitionRule, transitionCh chan<- TransitionRequest, getStorage StorageProviderForCriterion) *Listener {
	return &Listener{
		projectRoot:    projectRoot,
		wal:            wal,
		rules:          rules,
		transitionCh:   transitionCh,
		getStorage:     getStorage,
		satisfied:      make(map[string]struct{}),
		fired:          make(map[string]struct{}),
		checkpointPath: wal.CheckpointPath(),
	}
}

// Run runs the listener loop: load checkpoint, replay from WAL, process events, save checkpoint.
// Blocks until ctx is cancelled. Single goroutine per listener (concurrency guideline).
func (l *Listener) Run(ctx context.Context) error {
	logger := logging.NewEventLogger(ctx)
	if err := l.loadCheckpoint(); err != nil {
		logging.FluentEvent(logger).Debug("Lifecycle listener: load checkpoint failed, starting from seq 0").
			WithError(err).
			Log()
	}
	poll := WALIdlePollMin
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		l.mu.Lock()
		cursor := l.cursor
		l.mu.Unlock()
		var replayed int
		next, err := l.wal.ReplayFromCursor(cursor, func(ev *LifecycleEvent) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := l.processEvent(ev); err != nil {
				return err
			}
			replayed++
			return nil
		})
		if err != nil {
			logging.FluentEvent(logger).Debug("Lifecycle listener: replay failed").
				WithError(err).
				Log()
			return err
		}
		l.mu.Lock()
		l.cursor = next
		l.mu.Unlock()
		if replayed > 0 {
			_ = l.saveCheckpoint()
			poll = WALIdlePollMin
		} else {
			poll = NextWALIdlePoll(poll)
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			timer.Stop()
		}
	}
}

func (l *Listener) processEvent(ev *LifecycleEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch ev.EventType {
	case EventTypeCriterionSatisfied:
		key := ev.CriterionID + ":" + ev.scopeKey()
		l.satisfied[key] = struct{}{}
		l.tryFireRulesLocked(ev.CriterionID, ev.scopeKey(), ev.Scope)
	case EventTypeStatusTransition:
		// When a criteria transitions to a status that meets the gate, check if the parent milestone/backlog item criteria are satisfied.
		if ev.Kind == objects.KindCriteria && CriterionStatusMeetsMilestoneGateForMilestone(ev.ToStatus) {
			goroutinelabels.NewGoroutine("lifecycle_propagation", "propagate criteria transition to parents").StartSimple(func() {
				ctx := pkgctx.NewSystemContext()
				TryEmitForMilestonesContainingCriterion(ctx, l.projectRoot, ev.ID, l.getStorage)
				TryEmitForBacklogItemsContainingCriterion(ctx, l.projectRoot, ev.ID, l.getStorage)
			})
		}
	}
	return nil
}

func (l *Listener) tryFireRulesLocked(criterionID, scopeKey string, scope map[string]string) {
	for _, rule := range l.rules {
		if !rule.RuleMatch(criterionID, scopeKey, scope) {
			continue
		}
		objID := rule.ObjectID(scope)
		if objID == emptyValue {
			continue
		}
		fireKey := rule.CriterionID + ":" + scopeKey + "->" + rule.Kind + ":" + objID
		if _, already := l.fired[fireKey]; already {
			continue
		}
		select {
		case l.transitionCh <- TransitionRequest{Kind: rule.Kind, ID: objID, ToStatus: rule.ToStatus}:
			// Mark fired only after enqueue so a full channel can retry on the next WAL poll.
			l.fired[fireKey] = struct{}{}
		default:
			// channel full; don't block listener (retry on later events / polls)
		}
	}
}

func (l *Listener) loadCheckpoint() error {
	b, err := fileutil.ReadFile(l.checkpointPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	cursor, err := walutil.ParseReplayCursorCheckpoint(b)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.cursor = cursor
	l.mu.Unlock()
	return nil
}

func (l *Listener) saveCheckpoint() error {
	l.mu.Lock()
	cursor := l.cursor
	l.mu.Unlock()
	return fileutil.WriteFile(l.checkpointPath, walutil.FormatReplayCursorCheckpoint(cursor), paths.FilePerm600)
}

// AppendCriterionSatisfied appends a CriterionSatisfied event to the WAL (e.g. from lifecycle hook or job).
// Call from the same process that holds the WAL so Append is safe.
func AppendCriterionSatisfied(wal *LifecycleEventWAL, criterionID string, scope map[string]string) error {
	ev := &LifecycleEvent{
		EventType:   EventTypeCriterionSatisfied,
		CriterionID: criterionID,
		Scope:       scope,
	}
	return wal.Append(ev)
}
