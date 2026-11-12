package community

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

var (
	ErrInvalidTopic     = errors.New("invalid event topic format")
	ErrInvalidEvent     = errors.New("event payload or id is invalid")
	ErrSubscriberFailed = errors.New("subscriber handler panicked or returned an unhandled error")
	ErrMeshClosed       = errors.New("event mesh is closed")
)

var topicRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.\*]+$`)

// ProviderEvent represents an asynchronous event emitted across the provider mesh.
type ProviderEvent struct {
	ID        string                 `json:"id"`
	Topic     string                 `json:"topic"`
	Source    string                 `json:"source"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp time.Time              `json:"timestamp"`
}

// Validate verifies structural constraints on the event.
func (e *ProviderEvent) Validate() error {
	if e == nil {
		return ErrInvalidEvent
	}
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("%w: missing event id", ErrInvalidEvent)
	}
	if strings.TrimSpace(e.Topic) == "" || !topicRegex.MatchString(e.Topic) || strings.Contains(e.Topic, "*") {
		return fmt.Errorf("%w: invalid concrete publish topic %q", ErrInvalidTopic, e.Topic)
	}
	if e.Payload == nil {
		e.Payload = make(map[string]interface{})
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	return nil
}

type subscriberRegistration struct {
	id      string
	pattern string
	isWild  bool
	prefix  string
	handler func(ctx context.Context, event ProviderEvent) error
}

func (s *subscriberRegistration) matches(topic string) bool {
	if s.pattern == topic {
		return true
	}
	if s.isWild && strings.HasPrefix(topic, s.prefix) {
		return true
	}
	return false
}

// ProviderEventMesh provides dynamic topic pub/sub routing with isolated error boundaries.
type ProviderEventMesh struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriberRegistration
	nextSubID   uint64
	closed      bool
}

// NewProviderEventMesh creates a new initialized event mesh.
func NewProviderEventMesh() *ProviderEventMesh {
	return &ProviderEventMesh{
		subscribers: make(map[string]*subscriberRegistration),
	}
}

// Subscribe registers an event listener for a specific topic or wildcard prefix pattern.
func (m *ProviderEventMesh) Subscribe(pattern string, handler func(ctx context.Context, event ProviderEvent) error) (unsubscribe func(), err error) {
	if handler == nil {
		return nil, fmt.Errorf("%w: handler function cannot be nil", ErrSubscriberFailed)
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || !topicRegex.MatchString(pattern) {
		return nil, fmt.Errorf("%w: invalid subscription pattern %q", ErrInvalidTopic, pattern)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrMeshClosed
	}

	m.nextSubID++
	subID := fmt.Sprintf("sub-%d", m.nextSubID)

	isWild := strings.HasSuffix(pattern, ".*")
	prefix := ""
	if isWild {
		prefix = strings.TrimSuffix(pattern, ".*") + "."
	} else if pattern == "*" {
		isWild = true
		prefix = ""
	}

	reg := &subscriberRegistration{
		id:      subID,
		pattern: pattern,
		isWild:  isWild,
		prefix:  prefix,
		handler: handler,
	}

	m.subscribers[subID] = reg

	unsub := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.subscribers, subID)
	}

	return unsub, nil
}

// Publish broadcasts an event asynchronously to all matching subscribers with error and panic isolation.
func (m *ProviderEventMesh) Publish(ctx context.Context, event ProviderEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrMeshClosed
	}

	var targets []*subscriberRegistration
	for _, sub := range m.subscribers {
		if sub.matches(event.Topic) {
			targets = append(targets, sub)
		}
	}
	m.mu.RUnlock()

	if len(targets) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	for _, target := range targets {
		sub := target
		wg.Add(1)

		goroutinelabels.NewGoroutine("provider_eventmesh_dispatch", "dispatch provider event to subscriber").StartSimple(func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					// Isolate panic so sibling subscribers proceed normally
					_ = r
				}
			}()

			_ = sub.handler(ctx, event)
		})
	}

	wg.Wait()
	return nil
}

// SubscriberCount returns the active number of registered subscribers.
func (m *ProviderEventMesh) SubscriberCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subscribers)
}

// Close closes the event mesh and invalidates subscriptions.
func (m *ProviderEventMesh) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.subscribers = make(map[string]*subscriberRegistration)
	return nil
}
