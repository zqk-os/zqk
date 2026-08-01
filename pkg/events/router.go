package events

import "sync"

type EventType uint64

const (
	EventTypeUnknown             EventType = 0
	EventTypeSuspensionTriggered EventType = 1 << 0
	// EventTypeObjectMutated is emitted by the storage layer on any object state change.
	EventTypeObjectMutated EventType = 1 << 1
)

// Shape is the message published to the event router on object mutations.
// It carries enough information for localized policy inspectors to match
// without hitting the graph database.
type Shape struct {
	Type      EventType
	TargetID  string
	SourceID  string
	Kind      string // Object kind (e.g. "goal", "milestone")
	Namespace string // Namespace (e.g. "zqk:kernel")
	Status    string // Current status after the mutation
	Payload   any    // Optional opaque data for downstream consumers
}

type Inspector interface {
	Inspect(s Shape) bool
}

type BitmaskInspector struct {
	Mask EventType
}

func NewBitmaskInspector(mask EventType) *BitmaskInspector {
	return &BitmaskInspector{Mask: mask}
}

func (b *BitmaskInspector) Inspect(s Shape) bool {
	return s.Type&b.Mask != 0
}

// Router dispatches Shape messages to subscribers matched by their Inspector.
// cascadeInspectors run on every Publish call without needing a channel;
// they emit cascading shapes (e.g. SuspensionTriggered) directly into the router.
type Router struct {
	mu                sync.RWMutex
	subscribers       map[Inspector][]chan<- Shape
	cascadeInspectors []*PolicyInspector
}

func NewRouter() *Router {
	return &Router{
		subscribers: make(map[Inspector][]chan<- Shape),
	}
}

func (r *Router) Subscribe(inspector Inspector, ch chan<- Shape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscribers[inspector] = append(r.subscribers[inspector], ch)
}

// Publish delivers the shape to all matching subscribers and runs cascading policy inspectors.
func (r *Router) Publish(s Shape) {
	// Run policy cascade inspectors first (they may emit SuspensionTriggered shapes).
	r.mu.RLock()
	cascades := r.cascadeInspectors
	r.mu.RUnlock()
	for _, pi := range cascades {
		pi.Inspect(s)
	}

	// Deliver to channel-based subscribers.
	r.mu.RLock()
	defer r.mu.RUnlock()
	for inspector, channels := range r.subscribers {
		if inspector.Inspect(s) {
			for _, ch := range channels {
				select {
				case ch <- s:
				default:
				}
			}
		}
	}
}

// Unsubscribe removes a specific channel from an inspector's subscription list.
func (r *Router) Unsubscribe(inspector Inspector, ch chan<- Shape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	channels := r.subscribers[inspector]
	for i, c := range channels {
		if c == ch {
			r.subscribers[inspector] = append(channels[:i], channels[i+1:]...)
			return
		}
	}
}

// CascadePublisher is implemented by any type that can receive and re-publish shapes.
type CascadePublisher interface {
	Publish(s Shape)
}

// SubscribeWithCascade wires a PolicyInspector so that when it fires, it
// re-publishes a SuspensionTriggered shape to the given CascadePublisher.
// This is the core of the O(1) cascading suspension mechanism.
func (r *Router) SubscribeWithCascade(inspector *PolicyInspector, cascade CascadePublisher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inspector.cascade = cascade
	// Register under the router's subscriber map using the inspector itself as a sentinel channel source.
	// The PolicyInspector handles its own delivery via the cascade path, not a channel.
	r.cascadeInspectors = append(r.cascadeInspectors, inspector)
}

// PolicyInspectorConfig configures a localized policy inspector.
type PolicyInspectorConfig struct {
	Kind               string   // Object kind to watch (empty = all kinds)
	Namespace          string   // Namespace filter (empty = all namespaces)
	DisallowedStatuses []string // Status values that trigger a suspension
}

// PolicyInspector is a localized inspector that uses bitmask-fast matching
// on kind/namespace and fires a SuspensionTriggered cascade when a policy
// violation is detected. It replaces monolithic graph traversal with an
// application-layer O(1) check.
type PolicyInspector struct {
	config  PolicyInspectorConfig
	cascade CascadePublisher
}

// NewPolicyInspector creates a new PolicyInspector from the given config.
func NewPolicyInspector(cfg PolicyInspectorConfig) *PolicyInspector {
	return &PolicyInspector{config: cfg}
}

// Inspect returns true if the shape violates this inspector's policy.
func (p *PolicyInspector) Inspect(s Shape) bool {
	if p.config.Kind != "" && s.Kind != p.config.Kind {
		return false
	}
	if p.config.Namespace != "" && s.Namespace != p.config.Namespace {
		return false
	}
	for _, disallowed := range p.config.DisallowedStatuses {
		if s.Status == disallowed {
			if p.cascade != nil {
				p.cascade.Publish(Shape{
					Type:      EventTypeSuspensionTriggered,
					TargetID:  s.TargetID,
					SourceID:  s.SourceID,
					Kind:      s.Kind,
					Namespace: s.Namespace,
				})
			}
			return true
		}
	}
	return false
}
