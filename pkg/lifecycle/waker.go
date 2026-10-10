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

// GlobalJobWakerRegistry is the package-level instance of the singleton job waker registry.
var GlobalJobWakerRegistry = GetGlobalJobWakerRegistry()

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

func (r *JobWakerRegistry) registerChannel(bufferSize int, assign func(ch chan *LifecycleEvent)) (<-chan *LifecycleEvent, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return newClosedWakerChannel()
	}

	ch := make(chan *LifecycleEvent, bufferSize)
	assign(ch)

	return ch, func() {
		r.removeChannel(ch)
	}
}

// Register registers interest in a specific scheduler job ID.
// It returns a receive-only channel and a cleanup function to unregister.
func (r *JobWakerRegistry) Register(jobID string) (<-chan *LifecycleEvent, func()) {
	return r.registerChannel(1, func(ch chan *LifecycleEvent) {
		r.subscribers[jobID] = append(r.subscribers[jobID], ch)
	})
}

// RegisterWildcard registers interest in all scheduler job callback events.
func (r *JobWakerRegistry) RegisterWildcard() (<-chan *LifecycleEvent, func()) {
	return r.registerChannel(16, func(ch chan *LifecycleEvent) {
		r.wildcardSubscribers = append(r.wildcardSubscribers, ch)
	})
}

func (r *JobWakerRegistry) removeChannel(target chan *LifecycleEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for jobID, subs := range r.subscribers {
		for i, ch := range subs {
			if ch == target {
				r.subscribers[jobID] = append(subs[:i], subs[i+1:]...)
				if len(r.subscribers[jobID]) == 0 {
					delete(r.subscribers, jobID)
				}
				return
			}
		}
	}

	for i, ch := range r.wildcardSubscribers {
		if ch == target {
			r.wildcardSubscribers = append(r.wildcardSubscribers[:i], r.wildcardSubscribers[i+1:]...)
			return
		}
	}
}

// Unregister removes a registered channel for a specific job ID or wildcard subscriber.
func (r *JobWakerRegistry) Unregister(jobID string, target <-chan *LifecycleEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if subs, ok := r.subscribers[jobID]; ok {
		for i, ch := range subs {
			if (<-chan *LifecycleEvent)(ch) == target {
				r.subscribers[jobID] = append(subs[:i], subs[i+1:]...)
				if len(r.subscribers[jobID]) == 0 {
					delete(r.subscribers, jobID)
				}
				return
			}
		}
	}

	for i, ch := range r.wildcardSubscribers {
		if (<-chan *LifecycleEvent)(ch) == target {
			r.wildcardSubscribers = append(r.wildcardSubscribers[:i], r.wildcardSubscribers[i+1:]...)
			return
		}
	}
}

// Dispatch broadcasts a scheduler_callback or mutation event to all matching job wakers.
// Delivery is non-blocking to protect the caller from lagging consumers.
func (r *JobWakerRegistry) Dispatch(ev *LifecycleEvent) int {
	if ev == nil {
		return 0
	}
	if ev.EventType != "" && ev.EventType != EventTypeSchedulerCallback && ev.EventType != EventTypeStatusTransition {
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

	ch, unregister := r.Register(jobID)
	defer unregister()

	pollDone := make(chan struct{})
	goroutinelabels.NewGoroutine("lifecycle_waker_wal_poll", "poll WAL for scheduler callback").StartSimple(func() {
		defer close(pollDone)
		PollLifecycleWAL(pollCtx, projectRoot, pollInterval, func(ev *LifecycleEvent) {
			if ev != nil && ev.EventType == EventTypeSchedulerCallback {
				r.Dispatch(ev)
			}
		}, nil)
	})
	defer func() {
		cancelPoll()
		<-pollDone
	}()

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

// AwaitJobWithWAL awaits a job using the global waker registry and WAL replay.
func AwaitJobWithWAL(
	ctx context.Context,
	projectRoot string,
	jobID string,
	pollInterval time.Duration,
) (*LifecycleEvent, error) {
	return GetGlobalJobWakerRegistry().AwaitJobWithWAL(ctx, projectRoot, jobID, pollInterval)
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
