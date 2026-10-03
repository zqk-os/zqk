// Package lifecycle: updater consumes transition requests and applies them (cache-then-disk).
// Uses a single worker per project root to preserve ordering and bounded concurrency.

package lifecycle

import (
	"context"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/rollback"
	"github.com/zqk-os/zqk/pkg/storage"
)

// StorageProvider returns storage for a project root (e.g. from CLI cache). Nil if not available.
type StorageProvider func(projectRoot string) (storage.ObjectStorageProvider, bool)

// Updater consumes TransitionRequests and applies them via storage (Update).
// Run one updater worker per project root. When write-behind is available, can be extended to
// EnqueueStatusUpdate for cache-then-disk; for now uses storage.Update.
type Updater struct {
	projectRoot  string
	getStorage   StorageProvider
	transitionCh <-chan TransitionRequest
	logger       *logging.EventLogger
}

// NewUpdater creates an updater that reads from transitionCh and applies via getStorage(projectRoot).
func NewUpdater(projectRoot string, getStorage StorageProvider, transitionCh <-chan TransitionRequest) *Updater {
	return &Updater{
		projectRoot:  projectRoot,
		getStorage:   getStorage,
		transitionCh: transitionCh,
		logger:       logging.NewEventLogger(context.Background()),
	}
}

// Run consumes transition requests and applies each via storage.Update. Blocks until ctx is cancelled.
// Single goroutine (bounded concurrency). See docs/architecture/LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md.
func (u *Updater) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case req, ok := <-u.transitionCh:
			if !ok {
				return nil
			}
			if err := u.apply(ctx, req); err != nil {
				logging.FluentEvent(u.logger).Debug("Lifecycle updater: apply transition failed").
					Kind(req.Kind).
					ObjectID(req.ID).
					String("to_status", req.ToStatus).
					WithError(err).
					Log()
			}
		}
	}
}

func resolveStorageProvider(getStorage StorageProvider, projectRoot string) (storage.ObjectStorageProvider, bool) {
	if projectRoot == emptyValue || getStorage == nil {
		return nil, false
	}
	provider, ok := getStorage(projectRoot)
	if !ok {
		return nil, false
	}
	return nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
}

func (u *Updater) apply(ctx context.Context, req TransitionRequest) error {
	provider, ok := resolveStorageProvider(u.getStorage, u.projectRoot)
	if !ok {
		return nil
	}
	secCtx := pkgctx.NewSystemSecurityContext()

	// Capture rollback point for status-relevant graph before applying (if capture enabled).
	refs := StatusRelevantRefs(ctx, provider, req)
	states, err := rollback.SnapshotObjectStates(ctx, provider, refs)
	if err != nil {
		logging.FluentEvent(u.logger).Debug("Lifecycle updater: rollback snapshot failed (continuing)").
			Kind(req.Kind).
			ObjectID(req.ID).
			WithError(err).
			Log()
	}
	if err == nil && len(states) > 0 && u.projectRoot != emptyValue {
		scopeID := req.Kind + ":" + req.ID + ":" + req.ToStatus
		if _, capErr := rollback.Capture(u.projectRoot, rollback.ScopeTypeLifecycle, scopeID, func() ([]rollback.ObjectState, error) { return states, nil }); capErr != nil {
			logging.FluentEvent(u.logger).Debug("Lifecycle updater: rollback capture failed (continuing)").
				String("scope_id", scopeID).
				WithError(capErr).
				Log()
		} else {
			goroutinelabels.NewGoroutine("rollback_retain", "retaining rollback point").
				StartSimple(func() { _ = rollback.Retain(u.projectRoot) })
		}
	}

	updates := map[string]any{objects.FieldKeyStatus: req.ToStatus}
	opCtx := completionOverrideContext(ctx, secCtx, req)

	// Apply any status-transition compute hooks (e.g., effort variance)
	ApplyComputeHooks(opCtx, provider, secCtx, req, updates)

	updErr := provider.Update(opCtx, secCtx, req.ID, updates)
	if u.projectRoot != emptyValue {
		if updErr != nil {
			metrics.AppendProcessLifecycleJSONL(u.projectRoot, map[string]any{
				objects.FieldKeyEventType: metrics.ProcessLifecycleEventLifecycleTransitionFailed,
				objects.FieldKeyKind:      req.Kind,
				objects.FieldKeyID:        req.ID,
				"to_status":               req.ToStatus,
				"error_detail":            metrics.TruncateProcessLifecycleDetail(updErr.Error(), 512),
			})
		} else {
			var variance float64
			if v, ok := updates[objects.FieldKeyEffortVariance].(float64); ok {
				variance = v
			}
			metrics.AppendProcessLifecycleJSONL(u.projectRoot, map[string]any{
				objects.FieldKeyEventType:      metrics.ProcessLifecycleEventLifecycleTransitionApplied,
				objects.FieldKeyKind:           req.Kind,
				objects.FieldKeyID:             req.ID,
				"to_status":                    req.ToStatus,
				objects.FieldKeyEffortVariance: variance,
			})
		}
	}
	return updErr
}

// completionOverrideContext arms audited DECIDE break_glass for lifecycle-driven completion.
//
// priority_plan → complete is dual (manual+auto): promote can take the hop when holds pass.
// The updater still arms break_glass so critical-kind DECIDE elevation succeeds for the
// auto shockwave path. Break-glass must not skip complete/criteria holds — storage always
// ValidateLifecycle; the validator still enforces those preconditions.
//
// Elevation is stamped with break_glass: DECIDE refuses break_glass on critical kinds
// unless the actor is elevated. The updater runs as the system account.
// TRACK: follow-up in kernel backlog
// First-class promote recipe: docs/architecture/LIFECYCLE_STATUS_ROLES.md § First-class promote.
func completionOverrideContext(ctx context.Context, secCtx *pkgctx.SecurityContext, req TransitionRequest) context.Context {
	if req.ToStatus != statusComplete {
		return ctx
	}
	if req.Kind != objects.KindBacklogItem && req.Kind != objects.KindPriorityPlan {
		return ctx
	}
	// Declared intent: completing a backlog_item / priority_plan rewrites a core blob by design,
	// so the reason is stated here instead of inferred from the caller's privilege.
	opCtx := pkgctx.WithAllowCoreObjectDelete(ctx)
	return pkgctx.WithLifecycleBreakGlass(opCtx, "lifecycle updater auto-complete transition")
}

// RunListenerAndUpdater starts the listener and updater in separate goroutines and returns a stop function.
// Uses one goroutine for the listener and one for the updater (bounded). transitionCh is created with buffer size 64.
func RunListenerAndUpdater(ctx context.Context, projectRoot string, wal *LifecycleEventWAL, getStorage StorageProvider) (stop func()) {
	transitionCh := make(chan TransitionRequest, 64)
	rules := DefaultTransitionRules()
	listener := NewListener(projectRoot, wal, rules, transitionCh, StorageProviderForCriterion(getStorage))
	updater := NewUpdater(projectRoot, getStorage, transitionCh)

	var wg sync.WaitGroup
	listenerCtx, cancel := context.WithCancel(ctx)
	wg.Add(1)
	goroutinelabels.NewGoroutine("lifecycle_listener", "running lifecycle listener").
		WithWaitGroup(&wg).
		StartSimple(func() { _ = listener.Run(listenerCtx) })

	wg.Add(1)
	goroutinelabels.NewGoroutine("lifecycle_updater", "running lifecycle updater").
		WithWaitGroup(&wg).
		StartSimple(func() { _ = updater.Run(listenerCtx) })

	stop = func() {
		cancel()
		// Listener exits on cancel; close channel so updater drains and exits
		close(transitionCh)

		done := make(chan struct{})
		goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
			StartSimple(func() {
				func() {
					wg.Wait()
					close(done)
				}()
			})
		select {
		case <-done:
		case <-time.After(15 * time.Second):
		}
	}
	return stop
}
