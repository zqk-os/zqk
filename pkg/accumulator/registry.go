package accumulator

import (
	"context"
	"sync"
)

// WALSubscriber defines an accumulator that can tail the lifecycle WAL in the background.
type WALSubscriber interface {
	StartBackgroundWALSubscriber(ctx context.Context, updateCh chan<- struct{})
}

// SubscriberFactory instantiates or attaches a WALSubscriber for a given project root.
type SubscriberFactory func(projectRoot string) WALSubscriber

var (
	registryMu sync.RWMutex
	registry   = make(map[string]SubscriberFactory)
)

// RegisterSubscriber registers an accumulator factory by name.
func RegisterSubscriber(name string, factory SubscriberFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

// RegisteredSubscribers returns the names of all registered accumulator subscriber factories.
func RegisteredSubscribers() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// StartAllSubscribers starts background WAL subscribers for all registered accumulators.
func StartAllSubscribers(ctx context.Context, projectRoot string) []WALSubscriber {
	if projectRoot == "" {
		return nil
	}
	registryMu.RLock()
	defer registryMu.RUnlock()

	subscribers := make([]WALSubscriber, 0, len(registry))
	for _, factory := range registry {
		sub := factory(projectRoot)
		if sub != nil {
			sub.StartBackgroundWALSubscriber(ctx, nil)
			subscribers = append(subscribers, sub)
		}
	}
	return subscribers
}
