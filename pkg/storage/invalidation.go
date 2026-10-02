package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// MutationOp represents the type of mutation operation on storage objects.
type MutationOp string

const (
	MutationOpPut    MutationOp = "put"
	MutationOpDelete MutationOp = "delete"
)

// MutationEvent encapsulates a mutation shockwave event emitted on CAS and object storage writes.
type MutationEvent struct {
	Op         MutationOp     `json:"op"`
	Kind       string         `json:"kind"`
	ID         string         `json:"id"`
	OldHash    string         `json:"old_hash,omitempty"`
	NewHash    string         `json:"new_hash,omitempty"`
	Path       string         `json:"path,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
	ObjectData map[string]any `json:"object_data,omitempty"`
}

// ObjectMutationEvent is an alias for MutationEvent for compatibility with downstream specs and test cases.
type ObjectMutationEvent = MutationEvent

// InvalidationSubscriber receives mutation shockwave events broadcast across the bus.
type InvalidationSubscriber interface {
	HandleMutation(ctx context.Context, event MutationEvent) error
}

// InvalidationSubscriberFunc allows a bare function to implement InvalidationSubscriber.
type InvalidationSubscriberFunc func(ctx context.Context, event MutationEvent) error

// HandleMutation calls the underlying function.
func (f InvalidationSubscriberFunc) HandleMutation(ctx context.Context, event MutationEvent) error {
	return f(ctx, event)
}

// InvalidationShockwaveBus is a thread-safe event dispatcher that broadcasts object mutation shockwaves
// to downstream subscribers (e.g. in-memory kind index maps, ParseCache, ListCache, and ObjectIDCache).
type InvalidationShockwaveBus struct {
	mu          sync.RWMutex
	subscribers []InvalidationSubscriber
}

// NewInvalidationShockwaveBus creates a new InvalidationShockwaveBus.
func NewInvalidationShockwaveBus() *InvalidationShockwaveBus {
	return &InvalidationShockwaveBus{
		subscribers: make([]InvalidationSubscriber, 0),
	}
}

var (
	globalInvalidationBus     *InvalidationShockwaveBus
	globalInvalidationBusOnce sync.Once
)

// GetGlobalInvalidationBus returns the process-wide InvalidationShockwaveBus singleton.
func GetGlobalInvalidationBus() *InvalidationShockwaveBus {
	globalInvalidationBusOnce.Do(func() {
		globalInvalidationBus = NewInvalidationShockwaveBus()
		globalInvalidationBus.Subscribe(newParseCacheSubscriber(GetGlobalParseCache()))
		globalInvalidationBus.Subscribe(newListCacheSubscriber())
	})
	return globalInvalidationBus
}

func (b *InvalidationShockwaveBus) withSubscriberLock(sub InvalidationSubscriber, fn func()) {
	if b == nil || sub == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	fn()
}

// Subscribe registers a subscriber on the bus.
func (b *InvalidationShockwaveBus) Subscribe(sub InvalidationSubscriber) {
	b.withSubscriberLock(sub, func() {
		b.subscribers = append(b.subscribers, sub)
	})
}

// isSameSubscriber safely compares two subscribers, handling function types and pointers without panic.
func isSameSubscriber(a, b InvalidationSubscriber) bool {
	if a == nil || b == nil {
		return a == b
	}
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)
	if va.Type() != vb.Type() {
		return false
	}
	switch va.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Func:
		return va.Pointer() == vb.Pointer()
	default:
		if va.Type().Comparable() {
			return a == b
		}
		return false
	}
}

// Unsubscribe removes a subscriber from the bus.
func (b *InvalidationShockwaveBus) Unsubscribe(sub InvalidationSubscriber) {
	b.withSubscriberLock(sub, func() {
		filtered := b.subscribers[:0]
		for _, s := range b.subscribers {
			if !isSameSubscriber(s, sub) {
				filtered = append(filtered, s)
			}
		}
		b.subscribers = filtered
	})
}

// SubscribersCount returns the current count of registered subscribers.
func (b *InvalidationShockwaveBus) SubscribersCount() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// ClearSubscribers removes all registered subscribers.
func (b *InvalidationShockwaveBus) ClearSubscribers() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = make([]InvalidationSubscriber, 0)
}

// Broadcast dispatches the mutation event to all registered subscribers.
// Fail-safe: errors from individual subscribers are logged using structured logging
// and do not prevent other subscribers from executing.
func (b *InvalidationShockwaveBus) Broadcast(ctx context.Context, event MutationEvent) {
	if b == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	b.mu.RLock()
	subs := make([]InvalidationSubscriber, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.RUnlock()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	for _, sub := range subs {
		if err := sub.HandleMutation(ctx, event); err != nil {
			logging.Fluent(logger).Warn("invalidation shockwave subscriber returned error").
				Kind(event.Kind).
				ObjectID(event.ID).
				String("op", string(event.Op)).
				WithError(err).
				Log()
		}
	}
}

// SubscribeInvalidation registers a subscriber to the global shockwave bus.
func SubscribeInvalidation(sub InvalidationSubscriber) {
	GetGlobalInvalidationBus().Subscribe(sub)
}

// UnsubscribeInvalidation removes a subscriber from the global shockwave bus.
func UnsubscribeInvalidation(sub InvalidationSubscriber) {
	GetGlobalInvalidationBus().Unsubscribe(sub)
}

// BroadcastInvalidation broadcasts a mutation event across the global shockwave bus.
func BroadcastInvalidation(ctx context.Context, event MutationEvent) {
	GetGlobalInvalidationBus().Broadcast(ctx, event)
}

// broadcastInvalidationShockwave converts operation and path details into a MutationEvent
// and broadcasts it synchronously across the shockwave bus.
func broadcastInvalidationShockwave(ctx context.Context, op, kind, id, path string, objectData map[string]any) {
	broadcastInvalidationShockwaveWithOldHash(ctx, op, kind, id, path, "", objectData)
}

// broadcastInvalidationShockwaveWithOldHash converts operation and path details into a MutationEvent
// with explicit oldHash tracking and broadcasts it synchronously across the shockwave bus.
func broadcastInvalidationShockwaveWithOldHash(ctx context.Context, op, kind, id, path, oldHash string, objectData map[string]any) {
	var mutationOp MutationOp
	switch op {
	case OpCreate, OpUpdate, string(MutationOpPut):
		mutationOp = MutationOpPut
	case OpDelete:
		mutationOp = MutationOpDelete
	default:
		mutationOp = MutationOp(op)
	}

	hash := ""
	if path != emptyValue {
		base := filepath.Base(path)
		if filecas.CasHashFilenameRe.MatchString(base) {
			hash = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}

	var newHash string
	if mutationOp == MutationOpPut {
		newHash = hash
	} else if mutationOp == MutationOpDelete {
		if oldHash == "" {
			oldHash = hash
		}
	}

	if oldHash != "" && oldHash != newHash {
		GetGlobalParseCache().Evict(oldHash)
		filecas.EvictCASBlob(oldHash)
	}

	event := MutationEvent{
		Op:         mutationOp,
		Kind:       kind,
		ID:         id,
		OldHash:    oldHash,
		NewHash:    newHash,
		Path:       path,
		Timestamp:  time.Now(),
		ObjectData: objectData,
	}

	BroadcastInvalidation(ctx, event)
}

// parseCacheSubscriber evicts AST parse cache entries upon mutation shockwave.
type parseCacheSubscriber struct {
	cache *ParseCache
}

func newParseCacheSubscriber(cache *ParseCache) *parseCacheSubscriber {
	return &parseCacheSubscriber{cache: cache}
}

func (s *parseCacheSubscriber) HandleMutation(ctx context.Context, event MutationEvent) error {
	if s == nil || s.cache == nil {
		return nil
	}
	if event.OldHash != "" {
		s.cache.Evict(event.OldHash)
	}
	if event.Op == MutationOpDelete && event.NewHash != "" {
		s.cache.Evict(event.NewHash)
	}
	return nil
}

// listCacheSubscriber invalidates list cache upon mutation shockwaves.
type listCacheSubscriber struct{}

func newListCacheSubscriber() *listCacheSubscriber {
	return &listCacheSubscriber{}
}

func (s *listCacheSubscriber) HandleMutation(ctx context.Context, event MutationEvent) error {
	if event.Kind != "" {
		InvalidateListCacheForKind(event.Kind)
	} else {
		InvalidateListCache()
	}
	return nil
}
