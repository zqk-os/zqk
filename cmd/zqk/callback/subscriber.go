package callback

import (
	"context"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Subscriber defines the interface for reactive listeners responding to callback entries.
type Subscriber interface {
	Name() string
	Notify(ctx context.Context, entry *CallbackEntry) error
}

// CallbackSubscriber is an alias for Subscriber for explicit semantic naming across event bus domains.
type CallbackSubscriber = Subscriber

// FuncSubscriber allows registering closure handlers as callback subscribers.
type FuncSubscriber struct {
	name string
	fn   func(ctx context.Context, entry *CallbackEntry) error
}

// NewFuncSubscriber creates a new FuncSubscriber with the given name and closure.
func NewFuncSubscriber(name string, fn func(ctx context.Context, entry *CallbackEntry) error) *FuncSubscriber {
	return &FuncSubscriber{name: name, fn: fn}
}

// Name returns the identifier of the subscriber.
func (f *FuncSubscriber) Name() string {
	return f.name
}

// Notify invokes the underlying closure for the callback entry.
func (f *FuncSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	if f.fn == nil {
		return nil
	}
	return f.fn(ctx, entry)
}

// FileLogSubscriber delegates logging to the Processor's file logger.
type FileLogSubscriber struct {
	processor *Processor
}

// NewFileLogSubscriber creates a subscriber that writes formatted callback logs.
func NewFileLogSubscriber(p *Processor) *FileLogSubscriber {
	return &FileLogSubscriber{processor: p}
}

// Name returns the subscriber name for file logging.
func (s *FileLogSubscriber) Name() string {
	return "file_log"
}

// Notify writes the formatted log entry via the parent processor.
func (s *FileLogSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	if s.processor == nil {
		return errfmt.Errorf("processor is nil")
	}
	return s.processor.writeLogEntryToFile(entry)
}

// KernelWALSubscriber synthesizes Knowledge Kernel WAL events from callback payloads.
type KernelWALSubscriber struct {
	projectRoot string
	wal         *lifecycle.LifecycleEventWAL
	mu          sync.Mutex
}

// NewKernelWALSubscriber creates a subscriber that appends synthesized events to LifecycleEventWAL.
func NewKernelWALSubscriber(projectRoot string) *KernelWALSubscriber {
	return &KernelWALSubscriber{projectRoot: projectRoot}
}

// Name returns the identifier for the kernel WAL subscriber.
func (s *KernelWALSubscriber) Name() string {
	return "kernel_wal"
}

func (s *KernelWALSubscriber) ensureWAL() (*lifecycle.LifecycleEventWAL, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wal != nil {
		return s.wal, nil
	}
	if s.projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root is empty")
	}
	wal, err := lifecycle.NewLifecycleEventWAL(s.projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to open lifecycle WAL").Wrap(err)
	}
	s.wal = wal
	return s.wal, nil
}

// Notify translates callback payloads into LifecycleEvents and persists them in the WAL.
func (s *KernelWALSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	if entry == nil || entry.Payload == nil {
		return nil
	}
	wal, err := s.ensureWAL()
	if err != nil {
		return err
	}
	event := s.synthesizeEvent(entry)
	if event == nil {
		return nil
	}
	if err := wal.Append(event); err != nil {
		return errfmt.Newf("failed to append lifecycle event to WAL").Wrap(err)
	}
	return wal.Sync()
}

func (s *KernelWALSubscriber) synthesizeEvent(entry *CallbackEntry) *lifecycle.LifecycleEvent {
	payload := entry.Payload
	cbType, _ := payload[objects.FieldKeyCallbackType].(string)
	jobID, _ := payload["job_id"].(string)
	objID, _ := payload["object_id"].(string)
	kind, _ := payload[objects.FieldKeyKind].(string)
	critID, _ := payload["criteria_id"].(string)
	toStatus, _ := payload["status"].(string)
	fromStatus, _ := payload["from_status"].(string)

	event := &lifecycle.LifecycleEvent{
		Ts: entry.Timestamp,
		Scope: map[string]string{
			"job_id":        jobID,
			"callback_type": cbType,
		},
	}
	if critID != emptyValue {
		event.EventType = lifecycle.EventTypeCriterionSatisfied
		event.CriterionID = critID
		event.ID = objID
		event.Kind = kind
		return event
	}
	if toStatus != emptyValue {
		event.EventType = lifecycle.EventTypeStatusTransition
		event.ID = objID
		event.Kind = kind
		event.FromStatus = fromStatus
		event.ToStatus = toStatus
		return event
	}
	event.EventType = lifecycle.EventTypeSchedulerCallback
	event.ID = jobID
	event.Kind = "scheduler_job"
	if success, ok := payload["success"].(bool); ok && success {
		event.ToStatus = "completed"
	} else {
		event.ToStatus = "failed"
	}
	return event
}

// MultiSubscriberDispatcher coordinates and fans out callback events across registered subscribers.
type MultiSubscriberDispatcher struct {
	mu          sync.RWMutex
	subscribers map[string]Subscriber
	logger      logging.Logger
}

// NewMultiSubscriberDispatcher creates an initialized dispatcher.
func NewMultiSubscriberDispatcher(logger logging.Logger) *MultiSubscriberDispatcher {
	return &MultiSubscriberDispatcher{
		subscribers: make(map[string]Subscriber),
		logger:      logger,
	}
}

// Register adds or updates a subscriber in the registry.
func (d *MultiSubscriberDispatcher) Register(sub Subscriber) {
	if sub == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subscribers[sub.Name()] = sub
}

// RegisterResilient wraps the given subscriber with adaptive backpressure, concurrency throttling, and circuit breaking before registering.
func (d *MultiSubscriberDispatcher) RegisterResilient(sub Subscriber, cfg ResilientSubscriberConfig) *ResilientSubscriber {
	if sub == nil {
		return nil
	}
	if cfg.Logger == nil {
		cfg.Logger = d.logger
	}
	resilient := NewResilientSubscriber(sub, cfg)
	d.Register(resilient)
	return resilient
}

// Unregister removes a subscriber by name.
func (d *MultiSubscriberDispatcher) Unregister(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.subscribers, name)
}

// Subscribers returns a snapshot slice of all registered subscribers.
func (d *MultiSubscriberDispatcher) Subscribers() []Subscriber {
	d.mu.RLock()
	defer d.mu.RUnlock()
	list := make([]Subscriber, 0, len(d.subscribers))
	for _, s := range d.subscribers {
		list = append(list, s)
	}
	return list
}

// Dispatch broadcasts the callback entry concurrently to all subscribers with isolated failure reporting.
func (d *MultiSubscriberDispatcher) Dispatch(ctx context.Context, entry *CallbackEntry) error {
	subs := d.Subscribers()
	if len(subs) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(subs))

	for _, sub := range subs {
		wg.Add(1)
		currentSub := sub
		goroutinelabels.NewGoroutine("callback_subscriber_dispatch", "dispatch callback entry to subscriber").StartSimple(func() {
			defer wg.Done()
			if err := currentSub.Notify(ctx, entry); err != nil {
				if d.logger != nil {
					d.logger.Warn("subscriber failed to handle callback", logging.String("subscriber", currentSub.Name()), logging.Error(err))
				}
				errCh <- errfmt.Errorf("subscriber %s: %w", currentSub.Name(), err)
			}
		})
	}

	wg.Wait()
	close(errCh)

	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errfmt.Errorf("callback dispatch errors: %s", strings.Join(errs, "; "))
	}
	return nil
}
