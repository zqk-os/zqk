package callback

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Default token bucket and flow control configuration parameters.
const (
	DefaultTokenBucketCapacity   int64   = 100
	DefaultTokenBucketRefillRate float64 = 50.0
	DefaultDeficitQuantum        int64   = 1024
	DefaultMaxDeficit            int64   = 65536
	DefaultMaxQueueDepth         int     = 1000
)

// Common flow control error definitions.
var (
	ErrLimiterClosed          = errfmt.Errorf("token bucket limiter is closed")
	ErrInvalidTokenRequest    = errfmt.Errorf("requested tokens must be greater than zero")
	ErrTokenRequestExceedsCap = errfmt.Errorf("requested tokens exceeds bucket capacity")
	ErrQueueClosed            = errfmt.Errorf("subscriber deficit queue is closed")
	ErrSubscriberExists       = errfmt.Errorf("subscriber already registered in flow control manager")
)

// TokenBucketConfig defines parameters for a TokenBucketLimiter.
type TokenBucketConfig struct {
	Capacity      int64
	RefillRate    float64 // Tokens per second
	InitialTokens int64
	StartEmpty    bool
	Clock         func() time.Time
}

// TokenBucketLimiter implements a thread-safe token bucket rate limiter
// with burst allowance and continuous time-elapsed replenishment.
type TokenBucketLimiter struct {
	mu            sync.Mutex
	capacity      int64
	refillRate    float64
	currentTokens float64
	lastRefill    time.Time
	clock         func() time.Time
	closed        bool
	closeCh       chan struct{}
	notifyCh      chan struct{}
	totalAcquired atomic.Int64
	totalRejected atomic.Int64
}

// NewTokenBucketLimiter initializes a rate limiter with the given configuration.
func NewTokenBucketLimiter(cfg TokenBucketConfig) (*TokenBucketLimiter, error) {
	if cfg.Capacity <= 0 {
		return nil, ErrInvalidConfiguration
	}
	if cfg.RefillRate < 0 {
		return nil, ErrInvalidConfiguration
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	initTokens := cfg.Capacity
	if cfg.InitialTokens > 0 {
		initTokens = cfg.InitialTokens
	}
	if cfg.StartEmpty {
		initTokens = 0
	}
	if initTokens > cfg.Capacity {
		initTokens = cfg.Capacity
	}
	return &TokenBucketLimiter{
		capacity:      cfg.Capacity,
		refillRate:    cfg.RefillRate,
		currentTokens: float64(initTokens),
		lastRefill:    clock(),
		clock:         clock,
		closeCh:       make(chan struct{}),
		notifyCh:      make(chan struct{}),
	}, nil
}

// Capacity returns the maximum token capacity.
func (tb *TokenBucketLimiter) Capacity() int64 {
	return tb.capacity
}

// RefillRate returns the configured continuous refill rate (tokens/sec).
func (tb *TokenBucketLimiter) RefillRate() float64 {
	return tb.refillRate
}

// AvailableTokens returns the instantaneous floor count of available tokens.
func (tb *TokenBucketLimiter) AvailableTokens() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked(tb.clock())
	return int64(math.Floor(tb.currentTokens))
}

// TryAcquire attempts to consume the specified number of tokens non-blockingly.
func (tb *TokenBucketLimiter) TryAcquire(tokens int64) bool {
	if tokens <= 0 || tokens > tb.capacity {
		return false
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.closed {
		return false
	}
	tb.refillLocked(tb.clock())
	reqTokens := float64(tokens)
	if tb.currentTokens >= reqTokens {
		tb.currentTokens -= reqTokens
		tb.totalAcquired.Add(tokens)
		return true
	}
	tb.totalRejected.Add(1)
	return false
}

// Acquire blocks until sufficient tokens are available or the context is cancelled.
func (tb *TokenBucketLimiter) Acquire(ctx context.Context, tokens int64) error {
	if tokens <= 0 {
		return ErrInvalidTokenRequest
	}
	if tokens > tb.capacity {
		return ErrTokenRequestExceedsCap
	}
	for {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		ch, ok, err := tb.attemptAcquire(tokens)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if errWait := tb.waitForTokens(ctx, tokens, ch); errWait != nil {
			return errWait
		}
	}
}

// attemptAcquire attempts to atomically acquire tokens, returning notification channel if unfulfilled.
func (tb *TokenBucketLimiter) attemptAcquire(tokens int64) (<-chan struct{}, bool, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.closed {
		return nil, false, ErrLimiterClosed
	}
	tb.refillLocked(tb.clock())
	req := float64(tokens)
	if tb.currentTokens >= req {
		tb.currentTokens -= req
		tb.totalAcquired.Add(tokens)
		return nil, true, nil
	}
	return tb.notifyCh, false, nil
}

// waitForTokens calculates sleep duration until refill and sleeps or waits on notifyCh.
func (tb *TokenBucketLimiter) waitForTokens(ctx context.Context, tokens int64, ch <-chan struct{}) error {
	tb.mu.Lock()
	if tb.closed {
		tb.mu.Unlock()
		return ErrLimiterClosed
	}
	needed := float64(tokens) - tb.currentTokens
	rate := tb.refillRate
	closeChan := tb.closeCh
	tb.mu.Unlock()

	if rate <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-closeChan:
			return ErrLimiterClosed
		case <-ch:
			return nil
		}
	}

	sleepSec := needed / rate
	if sleepSec < 0.0001 {
		sleepSec = 0.0001
	}
	timer := time.NewTimer(time.Duration(sleepSec * float64(time.Second)))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-closeChan:
		return ErrLimiterClosed
	case <-ch:
		return nil
	case <-timer.C:
		return nil
	}
}

// refillLocked updates available tokens according to elapsed continuous time.
func (tb *TokenBucketLimiter) refillLocked(now time.Time) {
	if tb.refillRate <= 0 {
		return
	}
	elapsed := now.Sub(tb.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	tb.currentTokens += elapsed * tb.refillRate
	capFloat := float64(tb.capacity)
	if tb.currentTokens > capFloat {
		tb.currentTokens = capFloat
	}
	tb.lastRefill = now
	tb.broadcastRefill()
}

// broadcastRefill cycles the notify channel to wake up waiting consumers.
func (tb *TokenBucketLimiter) broadcastRefill() {
	close(tb.notifyCh)
	tb.notifyCh = make(chan struct{})
}

// Close closes the limiter and unblocks waiting callers.
func (tb *TokenBucketLimiter) Close() error {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.closed {
		return nil
	}
	tb.closed = true
	close(tb.closeCh)
	tb.broadcastRefill()
	return nil
}

// Stats returns cumulative acquisition and rejection counts.
func (tb *TokenBucketLimiter) Stats() (acquired, rejected int64) {
	return tb.totalAcquired.Load(), tb.totalRejected.Load()
}

// DeficitQueueItem represents an enqueued callback payload with an associated cost.
type DeficitQueueItem struct {
	Entry        *CallbackEntry
	Cost         int64
	SubscriberID string
}

// DeficitSubscriberQueue represents a FIFO queue for a specific tenant/subscriber.
type DeficitSubscriberQueue struct {
	subscriberID  string
	items         []*DeficitQueueItem
	quantum       int64
	deficit       int64
	maxDeficit    int64
	maxDepth      int
	totalEnqueued int64
	totalDequeued int64
	totalDropped  int64
}

func resolvePositive[T ~int | ~int64](val, defaultVal T) T {
	if val <= 0 {
		return defaultVal
	}
	return val
}

func newSubscriberQueue(id string, quantum, maxDeficit int64, maxDepth int) *DeficitSubscriberQueue {
	return &DeficitSubscriberQueue{
		subscriberID: id,
		items:        make([]*DeficitQueueItem, 0, 16),
		quantum:      resolvePositive(quantum, DefaultDeficitQuantum),
		deficit:      0,
		maxDeficit:   resolvePositive(maxDeficit, DefaultMaxDeficit),
		maxDepth:     resolvePositive(maxDepth, DefaultMaxQueueDepth),
	}
}

// DeficitRoundRobinConfig configures DeficitRoundRobinQueue.
type DeficitRoundRobinConfig struct {
	DefaultQuantum    int64
	DefaultMaxDeficit int64
	DefaultMaxDepth   int
}

// DeficitRoundRobinQueue implements Deficit Round-Robin (DRR) fair queueing
// across multiple subscriber flows to prevent noisy-neighbor starvation.
type DeficitRoundRobinQueue struct {
	mu           sync.Mutex
	cfg          DeficitRoundRobinConfig
	subscribers  map[string]*DeficitSubscriberQueue
	activeList   []string
	currentIndex int
	closed       bool
	notifyCh     chan struct{}
}

// NewDeficitRoundRobinQueue initializes a fair queueing manager.
func NewDeficitRoundRobinQueue(cfg DeficitRoundRobinConfig) *DeficitRoundRobinQueue {
	return &DeficitRoundRobinQueue{
		cfg: DeficitRoundRobinConfig{
			DefaultQuantum:    resolvePositive(cfg.DefaultQuantum, DefaultDeficitQuantum),
			DefaultMaxDeficit: resolvePositive(cfg.DefaultMaxDeficit, DefaultMaxDeficit),
			DefaultMaxDepth:   resolvePositive(cfg.DefaultMaxDepth, DefaultMaxQueueDepth),
		},
		subscribers: make(map[string]*DeficitSubscriberQueue),
		activeList:  make([]string, 0),
		notifyCh:    make(chan struct{}),
	}
}

func (drr *DeficitRoundRobinQueue) withLock(fn func() error) error {
	drr.mu.Lock()
	defer drr.mu.Unlock()
	if drr.closed {
		return ErrQueueClosed
	}
	return fn()
}

// RegisterSubscriber adds a new subscriber flow with optional custom quantum.
func (drr *DeficitRoundRobinQueue) RegisterSubscriber(id string, quantum, maxDeficit int64, maxDepth int) error {
	if id == "" {
		return ErrInvalidConfiguration
	}
	return drr.withLock(func() error {
		if _, exists := drr.subscribers[id]; exists {
			return ErrSubscriberExists
		}
		drr.subscribers[id] = newSubscriberQueue(
			id,
			resolvePositive(quantum, drr.cfg.DefaultQuantum),
			resolvePositive(maxDeficit, drr.cfg.DefaultMaxDeficit),
			resolvePositive(maxDepth, drr.cfg.DefaultMaxDepth),
		)
		return nil
	})
}

// Enqueue adds an entry with a given byte/token cost to the target subscriber queue.
func (drr *DeficitRoundRobinQueue) Enqueue(subscriberID string, entry *CallbackEntry, cost int64) error {
	if subscriberID == "" || entry == nil {
		return ErrInvalidConfiguration
	}
	if cost <= 0 {
		cost = 1
	}
	return drr.withLock(func() error {
		sq, exists := drr.subscribers[subscriberID]
		if !exists {
			sq = newSubscriberQueue(subscriberID, drr.cfg.DefaultQuantum, drr.cfg.DefaultMaxDeficit, drr.cfg.DefaultMaxDepth)
			drr.subscribers[subscriberID] = sq
		}
		if len(sq.items) >= sq.maxDepth {
			sq.totalDropped++
			return ErrQueueFull
		}
		wasEmpty := len(sq.items) == 0
		sq.items = append(sq.items, &DeficitQueueItem{
			Entry:        entry,
			Cost:         cost,
			SubscriberID: subscriberID,
		})
		sq.totalEnqueued++
		if wasEmpty {
			drr.activeList = append(drr.activeList, subscriberID)
			drr.broadcastAvailable()
		}
		return nil
	})
}

// Dequeue extracts the next fair item according to DRR round-robin deficit allocation.
func (drr *DeficitRoundRobinQueue) Dequeue() (*DeficitQueueItem, error) {
	drr.mu.Lock()
	defer drr.mu.Unlock()

	if drr.closed {
		return nil, ErrQueueClosed
	}
	return drr.stepDRRLocked()
}

// stepDRRLocked advances through active queues, allocating quanta and finding an eligible item.
func (drr *DeficitRoundRobinQueue) stepDRRLocked() (*DeficitQueueItem, error) {
	rounds := 0
	maxRounds := len(drr.activeList) * 2
	for len(drr.activeList) > 0 && rounds <= maxRounds {
		if drr.currentIndex >= len(drr.activeList) {
			drr.currentIndex = 0
		}
		subID := drr.activeList[drr.currentIndex]
		sq, ok := drr.subscribers[subID]
		if !ok || len(sq.items) == 0 {
			drr.removeActiveAt(drr.currentIndex)
			continue
		}

		// When visiting this subscriber in a round, add quantum if deficit was exhausted
		if sq.deficit < sq.items[0].Cost {
			sq.deficit += sq.quantum
			if sq.deficit > sq.maxDeficit {
				sq.deficit = sq.maxDeficit
			}
		}

		headItem := sq.items[0]
		if headItem.Cost <= sq.deficit {
			sq.deficit -= headItem.Cost
			sq.items = sq.items[1:]
			sq.totalDequeued++
			// If queue is now empty, reset deficit and remove from active list
			if len(sq.items) == 0 {
				sq.deficit = 0
				drr.removeActiveAt(drr.currentIndex)
			}
			if len(sq.items) > 0 && sq.items[0].Cost > sq.deficit {
				// Cannot serve next packet for this subscriber without more quantum: advance to next subscriber
				drr.currentIndex++
			}
			return headItem, nil
		}

		// Deficit still not enough even after quantum added: advance to next subscriber
		drr.currentIndex++
		rounds++
	}
	return nil, nil
}

// removeActiveAt removes the subscriber index from the active rotation list.
func (drr *DeficitRoundRobinQueue) removeActiveAt(idx int) {
	if idx < 0 || idx >= len(drr.activeList) {
		return
	}
	drr.activeList = append(drr.activeList[:idx], drr.activeList[idx+1:]...)
	if drr.currentIndex >= len(drr.activeList) && len(drr.activeList) > 0 {
		drr.currentIndex = 0
	}
}

// broadcastAvailable notifies any waiters that entries are available.
func (drr *DeficitRoundRobinQueue) broadcastAvailable() {
	close(drr.notifyCh)
	drr.notifyCh = make(chan struct{})
}

// DequeueBlocking blocks until an item is available or context is cancelled.
func (drr *DeficitRoundRobinQueue) DequeueBlocking(ctx context.Context) (*DeficitQueueItem, error) {
	for {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		item, ch, err := drr.tryDequeueOrWait()
		if err != nil {
			return nil, err
		}
		if item != nil {
			return item, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}

func (drr *DeficitRoundRobinQueue) tryDequeueOrWait() (*DeficitQueueItem, <-chan struct{}, error) {
	drr.mu.Lock()
	defer drr.mu.Unlock()

	if drr.closed {
		return nil, nil, ErrQueueClosed
	}
	item, err := drr.stepDRRLocked()
	if err != nil {
		return nil, nil, err
	}
	if item != nil {
		return item, nil, nil
	}
	return nil, drr.notifyCh, nil
}

// QueueDepth returns the current pending item count for a subscriber.
func (drr *DeficitRoundRobinQueue) QueueDepth(subscriberID string) int {
	drr.mu.Lock()
	defer drr.mu.Unlock()
	if sq, ok := drr.subscribers[subscriberID]; ok {
		return len(sq.items)
	}
	return 0
}

// Deficit returns the current accumulated deficit for a subscriber.
func (drr *DeficitRoundRobinQueue) Deficit(subscriberID string) int64 {
	drr.mu.Lock()
	defer drr.mu.Unlock()
	if sq, ok := drr.subscribers[subscriberID]; ok {
		return sq.deficit
	}
	return 0
}

// Close closes the queue and awakens blocking waiters.
func (drr *DeficitRoundRobinQueue) Close() error {
	drr.mu.Lock()
	defer drr.mu.Unlock()
	if drr.closed {
		return nil
	}
	drr.closed = true
	drr.broadcastAvailable()
	return nil
}

// FlowControlConfig specifies parameters for FlowControlManager.
type FlowControlConfig struct {
	Limiter TokenBucketConfig
	Queue   DeficitRoundRobinConfig
}

// FlowControlManager coordinates token bucket rate limiting and deficit round robin
// fair queueing across event mesh subscribers.
type FlowControlManager struct {
	mu          sync.RWMutex
	limiter     *TokenBucketLimiter
	drrQueue    *DeficitRoundRobinQueue
	subscribers map[string]Subscriber
	closed      bool
}

// NewFlowControlManager constructs an integrated flow controller.
func NewFlowControlManager(cfg FlowControlConfig) (*FlowControlManager, error) {
	lim, err := NewTokenBucketLimiter(cfg.Limiter)
	if err != nil {
		return nil, err
	}
	drr := NewDeficitRoundRobinQueue(cfg.Queue)
	return &FlowControlManager{
		limiter:     lim,
		drrQueue:    drr,
		subscribers: make(map[string]Subscriber),
	}, nil
}

// Limiter returns the underlying token bucket rate limiter.
func (m *FlowControlManager) Limiter() *TokenBucketLimiter {
	return m.limiter
}

// DRRQueue returns the underlying deficit round-robin queue.
func (m *FlowControlManager) DRRQueue() *DeficitRoundRobinQueue {
	return m.drrQueue
}

// RegisterSubscriber attaches a subscriber to the flow control manager.
func (m *FlowControlManager) RegisterSubscriber(sub Subscriber, quantum, maxDeficit int64, maxDepth int) error {
	if sub == nil || sub.Name() == "" {
		return ErrInvalidConfiguration
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrLimiterClosed
	}
	name := sub.Name()
	if _, exists := m.subscribers[name]; exists {
		return ErrSubscriberExists
	}
	if err := m.drrQueue.RegisterSubscriber(name, quantum, maxDeficit, maxDepth); err != nil {
		return err
	}
	m.subscribers[name] = sub
	return nil
}

// Submit enqueues an event for a registered subscriber after verifying rate limit quota.
func (m *FlowControlManager) Submit(subscriberID string, entry *CallbackEntry, cost int64) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrLimiterClosed
	}
	_, exists := m.subscribers[subscriberID]
	m.mu.RUnlock()
	if !exists {
		return ErrSubscriberNotFound
	}
	if !m.limiter.TryAcquire(cost) {
		return ErrCreditExhausted
	}
	return m.drrQueue.Enqueue(subscriberID, entry, cost)
}

// DispatchNext dequeues the next fair item and dispatches it to its registered subscriber.
func (m *FlowControlManager) DispatchNext(ctx context.Context) (*CallbackEntry, error) {
	item, err := m.drrQueue.DequeueBlocking(ctx)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	m.mu.RLock()
	sub, exists := m.subscribers[item.SubscriberID]
	m.mu.RUnlock()
	if !exists {
		return item.Entry, ErrSubscriberNotFound
	}
	if errNotify := sub.Notify(ctx, item.Entry); errNotify != nil {
		return item.Entry, errNotify
	}
	return item.Entry, nil
}

// Close gracefully closes the flow control manager, limiter, and DRR queue.
func (m *FlowControlManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	errLim := m.limiter.Close()
	errDRR := m.drrQueue.Close()
	if errLim != nil {
		return errLim
	}
	return errDRR
}
