package lifecycle

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// JobWakerRegistry coordinates reactive, zero-idle notifications for background
// scheduler job lifecycle callbacks persisted in the Knowledge Kernel WAL.
type JobWakerRegistry struct {
	mu                  sync.RWMutex
	subscribers         map[string][]chan *LifecycleEvent
	wildcardSubscribers []chan *LifecycleEvent
	closed              bool
}

var (
	globalWakerRegistry     *JobWakerRegistry
	globalWakerRegistryOnce sync.Once
)

// NewJobWakerRegistry creates an isolated registry for reactive job wakers.
func NewJobWakerRegistry() *JobWakerRegistry {
	return &JobWakerRegistry{
		subscribers:         make(map[string][]chan *LifecycleEvent),
		wildcardSubscribers: make([]chan *LifecycleEvent, 0),
	}
}

// GetGlobalJobWakerRegistry returns the process-wide singleton job waker registry.
func GetGlobalJobWakerRegistry() *JobWakerRegistry {
	globalWakerRegistryOnce.Do(func() {
		globalWakerRegistry = NewJobWakerRegistry()
	})
	return globalWakerRegistry
}

// Name returns the identifier of the waker registry.
func (r *JobWakerRegistry) Name() string {
	return "job_waker_registry"
}

// Wake dispatches a replayed WAL lifecycle event to registered wakers.
func (r *JobWakerRegistry) Wake(ctx context.Context, ev *LifecycleEvent) error {
	r.Dispatch(ev)
	return nil
}

func newClosedWakerChannel() (<-chan *LifecycleEvent, func()) {
	closedCh := make(chan *LifecycleEvent)
	close(closedCh)
	return closedCh, func() {}
}

// Register registers interest in a specific scheduler job ID.
// It returns a receive-only channel and a cleanup function to unregister.
func (r *JobWakerRegistry) Register(jobID string) (<-chan *LifecycleEvent, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return newClosedWakerChannel()
	}

	ch := make(chan *LifecycleEvent, 1)
	r.subscribers[jobID] = append(r.subscribers[jobID], ch)

	unregister := func() {
		r.unregisterChannel(jobID, ch)
	}

	return ch, unregister
}

// RegisterWildcard registers interest in all scheduler job callback events.
func (r *JobWakerRegistry) RegisterWildcard() (<-chan *LifecycleEvent, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return newClosedWakerChannel()
	}

	ch := make(chan *LifecycleEvent, 16)
	r.wildcardSubscribers = append(r.wildcardSubscribers, ch)

	unregister := func() {
		r.unregisterWildcard(ch)
	}

	return ch, unregister
}

func (r *JobWakerRegistry) unregisterChannel(jobID string, target chan *LifecycleEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	subs := r.subscribers[jobID]
	for i, ch := range subs {
		if ch == target {
			r.subscribers[jobID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(r.subscribers[jobID]) == 0 {
		delete(r.subscribers, jobID)
	}
}

func (r *JobWakerRegistry) unregisterWildcard(target chan *LifecycleEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, ch := range r.wildcardSubscribers {
		if ch == target {
			r.wildcardSubscribers = append(r.wildcardSubscribers[:i], r.wildcardSubscribers[i+1:]...)
			break
		}
	}
}

// Dispatch broadcasts a scheduler_callback event to all matching job wakers.
// Delivery is non-blocking to protect the caller from lagging consumers.
func (r *JobWakerRegistry) Dispatch(ev *LifecycleEvent) int {
	if ev == nil || ev.EventType != EventTypeSchedulerCallback {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return 0
	}

	dispatched := 0
	if targets, exists := r.subscribers[ev.ID]; exists {
		for _, ch := range targets {
			select {
			case ch <- ev:
				dispatched++
			default:
			}
		}
	}

	for _, ch := range r.wildcardSubscribers {
		select {
		case ch <- ev:
			dispatched++
		default:
		}
	}

	return dispatched
}

// AwaitJob blocks until the specified jobID completes or the context expires.
// Returns the completed LifecycleEvent or a context cancellation error.
func (r *JobWakerRegistry) AwaitJob(ctx context.Context, jobID string) (*LifecycleEvent, error) {
	ch, unregister := r.Register(jobID)
	defer unregister()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case ev, ok := <-ch:
		if !ok || ev == nil {
			return nil, errfmt.Errorf("waker channel closed without event")
		}
		return ev, nil
	}
}

// AwaitJobWithWAL sets up continuous WAL polling while awaiting the job callback.
func (r *JobWakerRegistry) AwaitJobWithWAL(
	ctx context.Context,
	projectRoot string,
	jobID string,
	pollInterval time.Duration,
) (*LifecycleEvent, error) {
	pollCtx, cancelPoll := context.WithCancel(ctx)
	defer cancelPoll()

	goroutinelabels.NewGoroutine("lifecycle_waker_wal_poll", "poll WAL for scheduler callback").StartSimple(func() {
		PollLifecycleWAL(pollCtx, projectRoot, pollInterval, func(ev *LifecycleEvent) {
			if ev != nil && ev.EventType == EventTypeSchedulerCallback {
				r.Dispatch(ev)
			}
		}, nil)
	})

	return r.AwaitJob(ctx, jobID)
}

// Close closes the registry and cleans up all registered channels.
func (r *JobWakerRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}
	r.closed = true

	for _, list := range r.subscribers {
		for _, ch := range list {
			close(ch)
		}
	}
	r.subscribers = make(map[string][]chan *LifecycleEvent)

	for _, ch := range r.wildcardSubscribers {
		close(ch)
	}
	r.wildcardSubscribers = nil
}
