// Package lifecycle: listener reads the lifecycle event WAL, accumulates satisfied criteria,
// and when all criteria for a transition rule are met, sends the transition to the updater channel.

package lifecycle

import (
	"context"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/walutil"
)

// TransitionRequest is a request to apply a status transition (kind, id, to_status).
type TransitionRequest struct {
	Kind     string
	ID       string
	ToStatus string
}

// CallbackWaker defines a receiver that is signaled when a callback event is replayed from the WAL.
type CallbackWaker interface {
	Name() string
	Wake(ctx context.Context, ev *LifecycleEvent) error
}

// FuncCallbackWaker enables closure functions to act as callback wakers.
type FuncCallbackWaker struct {
	name string
	fn   func(ctx context.Context, ev *LifecycleEvent) error
}

// NewFuncCallbackWaker creates a new FuncCallbackWaker.
func NewFuncCallbackWaker(name string, fn func(ctx context.Context, ev *LifecycleEvent) error) *FuncCallbackWaker {
	return &FuncCallbackWaker{name: name, fn: fn}
}

// Name returns the identifier of the waker.
func (f *FuncCallbackWaker) Name() string {
	return f.name
}

// Wake invokes the underlying closure with the lifecycle event.
func (f *FuncCallbackWaker) Wake(ctx context.Context, ev *LifecycleEvent) error {
	if f.fn == nil {
		return nil
	}
	return f.fn(ctx, ev)
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
	wakers         map[string]CallbackWaker
	cursor         walutil.ReplayCursor
	mu             sync.Mutex
	checkpointPath string
}

// NewListener creates a listener that reads from wal, evaluates rules, and sends transitions to transitionCh.
func NewListener(projectRoot string, wal *LifecycleEventWAL, rules []TransitionRule, transitionCh chan<- TransitionRequest, getStorage StorageProviderForCriterion) *Listener {
	l := &Listener{
		projectRoot:    projectRoot,
		wal:            wal,
		rules:          rules,
		transitionCh:   transitionCh,
		getStorage:     getStorage,
		satisfied:      make(map[string]struct{}),
		fired:          make(map[string]struct{}),
		wakers:         make(map[string]CallbackWaker),
		checkpointPath: wal.CheckpointPath(),
	}
	return l
}

// RegisterWaker registers a callback waker on the listener.
func (l *Listener) RegisterWaker(waker CallbackWaker) {
	if waker == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.wakers == nil {
		l.wakers = make(map[string]CallbackWaker)
	}
	l.wakers[waker.Name()] = waker
}

// UnregisterWaker removes a callback waker by name.
func (l *Listener) UnregisterWaker(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.wakers, name)
}

// Wakers returns a snapshot of all registered callback wakers.
func (l *Listener) Wakers() []CallbackWaker {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.snapshotWakersLocked()
}

func (l *Listener) snapshotWakersLocked() []CallbackWaker {
	list := make([]CallbackWaker, 0, len(l.wakers))
	for _, w := range l.wakers {
		list = append(list, w)
	}
	return list
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
			if saveErr := l.saveCheckpoint(); saveErr != nil {
				logging.FluentEvent(logger).Debug("Lifecycle listener: save checkpoint failed").
					WithError(saveErr).
					Log()
			}
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
	case EventTypeSchedulerCallback:
		wakers := l.snapshotWakersLocked()
		if len(wakers) > 0 {
			evCopy := *ev
			goroutinelabels.NewGoroutine("lifecycle_waker_dispatch", "dispatch scheduler callback to wakers").StartSimple(func() {
				ctx := pkgctx.NewSystemContext()
				logger := logging.NewEventLogger(ctx)
				for _, waker := range wakers {
					if err := waker.Wake(ctx, &evCopy); err != nil {
						logging.FluentEvent(logger).Warn("Lifecycle listener: callback waker returned error").
							WithError(err).
							Log()
					}
				}
			})
		}
	}
	return nil
}

// DispatchCallbackNow synchronously dispatches a callback event to all registered wakers.
func (l *Listener) DispatchCallbackNow(ctx context.Context, ev *LifecycleEvent) []error {
	wakers := l.Wakers()
	var errs []error
	for _, waker := range wakers {
		if err := waker.Wake(ctx, ev); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
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
