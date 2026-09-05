package observer

import (
	"context"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

const (
	goroutineEmitterSubscriber = "observer_emitter_subscriber"
	goroutineEmitterPurpose    = "notifying observer event subscriber"
	goroutineConnectionCB      = "observer_connection_callback"
	goroutineConnectionPurpose = "notifying agent connection callback"
	payloadClientIDKey         = "client_id"
	payloadClientNameKey       = "client_name"
	payloadVersionKey          = "version"
	lockProfileSystem          = string(pkgctx.ProfileSystem)
)

// ObserverEventType identifies observer lifecycle and extraction events (BLI-813).
const (
	EventExtractStarted    = "observer.extract_started"
	EventExtractCompleted  = "observer.extract_completed"
	EventExtractFailed     = "observer.extract_failed"
	EventPopulateStarted   = "observer.populate_started"
	EventPopulateCompleted = "observer.populate_completed"
	EventPopulateFailed    = "observer.populate_failed"
)

// Agent connection events for automatic registration (BLI-857, BLI-858).
// Emitted by MCP when an unregistered agent connects; observer can subscribe and create accounts (BLI-859).
const (
	EventAgentConnectionStarted = "observer.agent_connection_started" // unregistered agent connected
)

// ObserverEvent is emitted by the observer for extract/populate lifecycle (BLI-813).
type ObserverEvent struct {
	Type        string        // EventExtractStarted, etc.
	ExtractID   string        // Run or batch id
	Dir         string        // Directory scanned
	EntityCount int           // Set on completed events
	Error       string        // Set on failed events
	Duration    time.Duration // Set on completed/failed
	Timestamp   time.Time
	Payload     map[string]any // Optional extra data
}

// ObserverEmitter emits observer events. Subscribers are notified asynchronously.
// Use for integration with event bus or logging (BLI-813).
// Concurrency: critical sections use concurrency.RunInLockWithLogger (see pkg/concurrency/README.md).
type ObserverEmitter struct {
	mu          sync.Mutex
	subscribers []func(context.Context, ObserverEvent)
}

// NewObserverEmitter returns a new emitter with no subscribers.
func NewObserverEmitter() *ObserverEmitter {
	return &ObserverEmitter{}
}

// Subscribe adds a callback that is invoked for each emitted event (non-blocking).
func (e *ObserverEmitter) Subscribe(fn func(context.Context, ObserverEvent)) {
	_ = concurrency.RunInLockWithLogger(&e.mu, LockNameObserverEmitterSubscribe, logging.GetLockLoggerFromProfile(lockProfileSystem), func() error {
		e.subscribers = append(e.subscribers, fn)
		return nil
	})
}

// Emit sends an event to all subscribers. Callers should not block; subscribers are called in goroutines.
func (e *ObserverEmitter) Emit(ctx context.Context, ev ObserverEvent) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	var subs []func(context.Context, ObserverEvent)
	_ = concurrency.RunInLockWithLogger(&e.mu, LockNameObserverEmitterEmitCopy, logging.GetLockLoggerFromProfile(lockProfileSystem), func() error {
		subs = make([]func(context.Context, ObserverEvent), len(e.subscribers))
		copy(subs, e.subscribers)
		return nil
	})
	bud := goroutinelabels.DefaultBudget()
	for _, fn := range subs {
		b := goroutinelabels.NewGoroutine(goroutineEmitterSubscriber, goroutineEmitterPurpose).
			WithContext(ctx)
		if bud != nil {
			b = b.WithBudget(bud)
		}
		b.StartSimple(func() { fn(ctx, ev) })
	}
}

// DefaultEmitter is the process-wide observer emitter. Set by observer CLI or tests.
var DefaultEmitter *ObserverEmitter

func init() {
	DefaultEmitter = NewObserverEmitter()
}

// AgentConnectionInfo is passed when an unregistered agent connects (BLI-857). Observer can create account (BLI-859).
type AgentConnectionInfo struct {
	ClientID   string
	ClientName string
	Version    string
}

// ConnectionEventCallback is invoked when MCP reports an agent connection (e.g. unregistered). BLI-858: observer subscribes.
type ConnectionEventCallback func(ctx context.Context, ev AgentConnectionInfo)

// Concurrency: subscription list and notify copy use RunInLockWithLogger (see pkg/concurrency/README.md).
var (
	connectionCallbacksMu sync.Mutex
	connectionCallbacks   []ConnectionEventCallback
)

// RegisterConnectionEventCallback adds a subscriber for agent connection events (BLI-858). Called by observer on startup.
func RegisterConnectionEventCallback(fn ConnectionEventCallback) {
	_ = concurrency.RunInLockWithLogger(&connectionCallbacksMu, LockNameObserverRegisterConnectionCallback, logging.GetLockLoggerFromProfile(lockProfileSystem), func() error {
		connectionCallbacks = append(connectionCallbacks, fn)
		return nil
	})
}

// NotifyAgentConnection is called by MCP when an unregistered agent connects (BLI-857). Notifies all registered callbacks.
func NotifyAgentConnection(ctx context.Context, info AgentConnectionInfo) {
	var cb []ConnectionEventCallback
	_ = concurrency.RunInLockWithLogger(&connectionCallbacksMu, LockNameObserverNotifyConnectionCopy, logging.GetLockLoggerFromProfile(lockProfileSystem), func() error {
		cb = make([]ConnectionEventCallback, len(connectionCallbacks))
		copy(cb, connectionCallbacks)
		return nil
	})
	bud := goroutinelabels.DefaultBudget()
	for _, fn := range cb {
		b := goroutinelabels.NewGoroutine(goroutineConnectionCB, goroutineConnectionPurpose).
			WithContext(ctx)
		if bud != nil {
			b = b.WithBudget(bud)
		}
		b.StartSimple(func() { fn(ctx, info) })
	}
	DefaultEmitter.Emit(ctx, ObserverEvent{
		Type: EventAgentConnectionStarted,
		Payload: map[string]any{
			payloadClientIDKey: info.ClientID, payloadClientNameKey: info.ClientName, payloadVersionKey: info.Version,
		},
	})
}
