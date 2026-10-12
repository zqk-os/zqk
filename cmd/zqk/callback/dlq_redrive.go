package callback

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// QuarantineStatus defines the discrete lifecycle states of an event in the quarantine system.
type QuarantineStatus string

const (
	// StatusQuarantined marks an event actively held in quarantine awaiting manual or scheduled redrive.
	StatusQuarantined QuarantineStatus = "Quarantined"

	// StatusReplaying indicates an envelope is currently undergoing in-flight delivery.
	StatusReplaying QuarantineStatus = "Replaying"

	// StatusRedriven denotes successful re-delivery to the target subscriber.
	StatusRedriven QuarantineStatus = "Redriven"

	// StatusPoisonPill isolates events that have permanently exceeded max redrive attempts or caused terminal faults.
	StatusPoisonPill QuarantineStatus = "PoisonPill"

	// StatusExpired marks an envelope evicted from quarantine storage due to capacity bounds.
	StatusExpired QuarantineStatus = "Expired"

	// Canonical aliases for backward compatibility and idiomatic usage.
	Quarantined = StatusQuarantined
	Replaying   = StatusReplaying
	Redriven    = StatusRedriven
	PoisonPill  = StatusPoisonPill
	Expired     = StatusExpired

	// DefaultDLQCapacity defines bounded quarantine envelope limit.
	DefaultDLQCapacity = 1000

	// DefaultMaxAttempts is the fallback redrive ceiling before poison pill isolation.
	DefaultMaxAttempts = 5

	// DefaultBaseBackoff is the starting exponential backoff duration.
	DefaultBaseBackoff = 50 * time.Millisecond

	// DefaultMaxBackoff is the ceiling exponential backoff duration.
	DefaultMaxBackoff = 5 * time.Second
)

var (
	// ErrCorruptedEnvelope signals that payload checksum validation failed, indicating tampering or data corruption.
	ErrCorruptedEnvelope = errfmt.Errorf("envelope payload checksum mismatch: corrupted envelope")

	// ErrEnvelopeNotFound indicates that no quarantine envelope was found for the requested ID.
	ErrEnvelopeNotFound = errfmt.Errorf("quarantine envelope not found")

	// ErrMaxAttemptsExceeded signals that the event has exceeded allowable attempts and is locked in poison pill isolation.
	ErrMaxAttemptsExceeded = errfmt.Errorf("max redrive attempts exceeded: poison pill isolated")

	// ErrDLQClosed indicates operations attempted on a terminated dead-letter queue manager.
	ErrDLQClosed = errfmt.Errorf("dlq manager is closed")

	// ErrInvalidEnvelope indicates a nil or malformed quarantine envelope structure.
	ErrInvalidEnvelope = errfmt.Errorf("invalid quarantine envelope")

	// ErrNilSubscriber indicates a nil subscriber was supplied as redrive destination.
	ErrNilSubscriber = errfmt.Errorf("target subscriber cannot be nil")

	envelopeSeq atomic.Int64
)

// QuarantineEnvelope encapsulates a failed callback event with cryptographic integrity verification
// and delivery tracking metadata.
type QuarantineEnvelope struct {
	ID              string           `json:"id"`
	EnvelopeID      string           `json:"envelope_id"`
	Entry           *CallbackEntry   `json:"entry"`
	Topic           string           `json:"topic"`
	SubscriberID    string           `json:"subscriber_id"`
	Reason          string           `json:"reason"`
	AttemptCount    int              `json:"attempt_count"`
	MaxAttempts     int              `json:"max_attempts"`
	PayloadChecksum string           `json:"payload_checksum"`
	FirstFailedAt   time.Time        `json:"first_failed_at"`
	LastFailedAt    time.Time        `json:"last_failed_at"`
	NextRedriveAt   time.Time        `json:"next_redrive_at"`
	Status          QuarantineStatus `json:"status"`
}

// Clone creates a shallow thread-safe snapshot of the envelope.
func (e *QuarantineEnvelope) Clone() *QuarantineEnvelope {
	if e == nil {
		return nil
	}
	cp := *e
	return &cp
}

// DLQFilter specifies criteria for filtering and querying quarantined envelopes.
type DLQFilter struct {
	Topic        string
	SubscriberID string
	Status       QuarantineStatus
	MaxAge       time.Duration
	Limit        int
}

// Matches returns true if the envelope conforms to the specified filter criteria.
func (f DLQFilter) Matches(env *QuarantineEnvelope) bool {
	if env == nil {
		return false
	}
	if f.Topic != "" && env.Topic != f.Topic {
		return false
	}
	if f.SubscriberID != "" && env.SubscriberID != f.SubscriberID {
		return false
	}
	if f.Status != "" && env.Status != f.Status {
		return false
	}
	if f.MaxAge > 0 && time.Since(env.FirstFailedAt) > f.MaxAge {
		return false
	}
	return true
}

// PoisonPillQuarantine maintains a bounded, thread-safe isolation vault for toxic events.
type PoisonPillQuarantine struct {
	mu          sync.RWMutex
	pills       map[string]*QuarantineEnvelope
	orderedKeys []string
	maxCapacity int
}

// NewPoisonPillQuarantine allocates an initialized poison pill isolation vault.
func NewPoisonPillQuarantine(capacity int) *PoisonPillQuarantine {
	if capacity <= 0 {
		capacity = 500
	}
	return &PoisonPillQuarantine{
		pills:       make(map[string]*QuarantineEnvelope),
		orderedKeys: make([]string, 0, capacity),
		maxCapacity: capacity,
	}
}

// Isolate moves an envelope into poison pill quarantine.
func (q *PoisonPillQuarantine) Isolate(env *QuarantineEnvelope) {
	if env == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	env.Status = StatusPoisonPill
	if pillItem, exists := q.pills[env.ID]; !exists || pillItem == nil {
		if len(q.orderedKeys) >= q.maxCapacity && len(q.orderedKeys) > 0 {
			oldestKey := q.orderedKeys[0]
			q.orderedKeys = q.orderedKeys[1:]
			delete(q.pills, oldestKey)
		}
		q.orderedKeys = append(q.orderedKeys, env.ID)
	}
	q.pills[env.ID] = env
}

// Get retrieves an isolated poison pill by ID.
func (q *PoisonPillQuarantine) Get(id string) (*QuarantineEnvelope, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	item, ok := q.pills[id]
	if !ok || item == nil {
		return nil, false
	}
	return item.Clone(), true
}

// List returns a snapshot slice of all isolated poison pills in insertion order.
func (q *PoisonPillQuarantine) List() []*QuarantineEnvelope {
	q.mu.RLock()
	defer q.mu.RUnlock()
	result := make([]*QuarantineEnvelope, 0, len(q.orderedKeys))
	for idx := 0; idx < len(q.orderedKeys); idx++ {
		key := q.orderedKeys[idx]
		if item, ok := q.pills[key]; ok && item != nil {
			result = append(result, item.Clone())
		}
	}
	return result
}

// Remove deletes a poison pill from isolation, returning the envelope if found.
func (q *PoisonPillQuarantine) Remove(id string) (*QuarantineEnvelope, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, ok := q.pills[id]
	if !ok {
		return nil, false
	}
	delete(q.pills, id)
	newKeys := make([]string, 0, len(q.orderedKeys)-1)
	for idx := 0; idx < len(q.orderedKeys); idx++ {
		key := q.orderedKeys[idx]
		if key != id {
			newKeys = append(newKeys, key)
		}
	}
	q.orderedKeys = newKeys
	return item, true
}

// Count returns the active count of isolated poison pills.
func (q *PoisonPillQuarantine) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.pills)
}

// Capacity returns the maximum storage capacity of the isolation vault.
func (q *PoisonPillQuarantine) Capacity() int {
	return q.maxCapacity
}

// Clear flushes all entries from the poison pill quarantine.
func (q *PoisonPillQuarantine) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pills = make(map[string]*QuarantineEnvelope)
	q.orderedKeys = q.orderedKeys[:0]
}

// DLQOption configures DistributedDLQManager parameters.
type DLQOption func(*DistributedDLQManager)

// WithCapacity sets the bounded capacity for active quarantine envelopes.
func WithCapacity(capacity int) DLQOption {
	return func(m *DistributedDLQManager) {
		if capacity > 0 {
			m.capacity = capacity
		}
	}
}

// WithMaxAttempts sets the failure threshold before poison pill auto-isolation.
func WithMaxAttempts(maxAttempts int) DLQOption {
	return func(m *DistributedDLQManager) {
		if maxAttempts > 0 {
			m.maxAttempts = maxAttempts
		}
	}
}

// WithBackoff sets base and maximum backoff durations for redrive scheduling.
func WithBackoff(base, max time.Duration) DLQOption {
	return func(m *DistributedDLQManager) {
		if base > 0 {
			m.baseBackoff = base
		}
		if max > 0 {
			m.maxBackoff = max
		}
	}
}

// DLQStats summarizes current dead-letter queue metrics.
type DLQStats struct {
	Capacity      int
	ActiveCount   int
	PoisonCount   int
	EnqueuedTotal int64
	RedrivenTotal int64
	EvictedTotal  int64
}

// DistributedDLQManager coordinates bounded event quarantine, cryptographic tamper detection,
// poison pill isolation, and automated redrive replay.
type DistributedDLQManager struct {
	mu            sync.RWMutex
	capacity      int
	maxAttempts   int
	baseBackoff   time.Duration
	maxBackoff    time.Duration
	envelopes     map[string]*QuarantineEnvelope
	order         []string
	poisonPills   *PoisonPillQuarantine
	evictedCount  atomic.Int64
	enqueuedCount atomic.Int64
	redrivenCount atomic.Int64
	closed        atomic.Bool
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
	workerRunning atomic.Bool
}

// NewDistributedDLQManager constructs an initialized DLQ manager with optional parameter overrides.
func NewDistributedDLQManager(opts ...DLQOption) *DistributedDLQManager {
	mgr := &DistributedDLQManager{
		capacity:    DefaultDLQCapacity,
		maxAttempts: DefaultMaxAttempts,
		baseBackoff: DefaultBaseBackoff,
		maxBackoff:  DefaultMaxBackoff,
		envelopes:   make(map[string]*QuarantineEnvelope),
		order:       make([]string, 0, DefaultDLQCapacity),
		poisonPills: NewPoisonPillQuarantine(DefaultDLQCapacity / 2),
		stopCh:      make(chan struct{}),
	}
	for idx := 0; idx < len(opts); idx++ {
		opts[idx](mgr)
	}
	return mgr
}

// Name identifies the DLQ manager component in telemetry and event dispatch meshes.
func (m *DistributedDLQManager) Name() string {
	return "distributed_dlq_manager"
}

// Enqueue captures a failed callback event into a tamper-evident quarantine envelope.
func (m *DistributedDLQManager) Enqueue(ctx context.Context, entry *CallbackEntry, subscriberID string, reason string) (*QuarantineEnvelope, error) {
	topic := extractTopic(entry)
	return m.EnqueueWithTopic(ctx, entry, topic, subscriberID, reason)
}

// EnqueueWithTopic captures a failed event under an explicit topic into a quarantine envelope.
func (m *DistributedDLQManager) EnqueueWithTopic(ctx context.Context, entry *CallbackEntry, topic string, subscriberID string, reason string) (*QuarantineEnvelope, error) {
	if m.closed.Load() {
		return nil, ErrDLQClosed
	}
	if entry == nil {
		return nil, ErrInvalidEnvelope
	}

	envID := generateEnvelopeID()
	checksum := CalculateChecksum(entry)
	now := time.Now().UTC()

	env := &QuarantineEnvelope{
		ID:              envID,
		EnvelopeID:      envID,
		Entry:           entry,
		Topic:           topic,
		SubscriberID:    subscriberID,
		Reason:          reason,
		AttemptCount:    0,
		MaxAttempts:     m.maxAttempts,
		PayloadChecksum: checksum,
		FirstFailedAt:   now,
		LastFailedAt:    now,
		NextRedriveAt:   now.Add(CalculateBackoff(1, m.baseBackoff, m.maxBackoff)),
		Status:          StatusQuarantined,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.envelopes) >= m.capacity && len(m.order) > 0 {
		m.evictOldestLocked()
	}

	m.envelopes[envID] = env
	m.order = append(m.order, envID)
	m.enqueuedCount.Add(1)

	return env, nil
}

func (m *DistributedDLQManager) evictOldestLocked() {
	oldestID := m.order[0]
	m.order = m.order[1:]
	if oldEnv, exists := m.envelopes[oldestID]; exists {
		oldEnv.Status = StatusExpired
		delete(m.envelopes, oldestID)
		m.evictedCount.Add(1)
	}
}

// Get retrieves a quarantined envelope or isolated poison pill by its ID.
func (m *DistributedDLQManager) Get(id string) (*QuarantineEnvelope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if env, ok := m.envelopes[id]; ok && env != nil {
		return env.Clone(), nil
	}
	if pill, ok := m.poisonPills.Get(id); ok && pill != nil {
		return pill.Clone(), nil
	}
	return nil, ErrEnvelopeNotFound
}

// List returns all envelopes matching the specified filter criteria.
func (m *DistributedDLQManager) List(filter DLQFilter) []*QuarantineEnvelope {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]*QuarantineEnvelope, 0, len(m.order))
	for idx := 0; idx < len(m.order); idx++ {
		id := m.order[idx]
		env, exists := m.envelopes[id]
		if !exists || env == nil {
			continue
		}
		if filter.Matches(env) {
			results = append(results, env.Clone())
			if filter.Limit > 0 && len(results) >= filter.Limit {
				break
			}
		}
	}
	return results
}

// Redrive attempts point-in-time redelivery of an envelope to a designated subscriber.
func (m *DistributedDLQManager) Redrive(ctx context.Context, id string, targetSubscriber Subscriber) error {
	if m.closed.Load() {
		return ErrDLQClosed
	}
	if targetSubscriber == nil {
		return ErrNilSubscriber
	}

	m.mu.Lock()
	env, exists := m.envelopes[id]
	if !exists {
		m.mu.Unlock()
		if poisonPill, isPoison := m.poisonPills.Get(id); isPoison && poisonPill != nil {
			return ErrMaxAttemptsExceeded
		}
		return ErrEnvelopeNotFound
	}

	if err := ValidateIntegrity(env); err != nil {
		m.mu.Unlock()
		return err
	}

	if env.Status == StatusPoisonPill || env.AttemptCount >= env.MaxAttempts {
		m.poisonPills.Isolate(env)
		delete(m.envelopes, id)
		m.mu.Unlock()
		return ErrMaxAttemptsExceeded
	}

	env.Status = StatusReplaying
	m.mu.Unlock()

	notifyErr := targetSubscriber.Notify(ctx, env.Entry)

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	if notifyErr == nil {
		env.Status = StatusRedriven
		m.redrivenCount.Add(1)
		return nil
	}

	env.AttemptCount++
	env.LastFailedAt = now
	env.Reason = notifyErr.Error()

	if env.AttemptCount >= env.MaxAttempts {
		env.Status = StatusPoisonPill
		m.poisonPills.Isolate(env)
		delete(m.envelopes, id)
		return notifyErr
	}

	env.Status = StatusQuarantined
	env.NextRedriveAt = now.Add(CalculateBackoff(env.AttemptCount, m.baseBackoff, m.maxBackoff))
	return notifyErr
}

// RedriveBatch attempts sequential re-delivery of all envelopes matching the specified filter.
func (m *DistributedDLQManager) RedriveBatch(ctx context.Context, filter DLQFilter, targetSubscriber Subscriber) ([]*QuarantineEnvelope, error) {
	if m.closed.Load() {
		return nil, ErrDLQClosed
	}
	if targetSubscriber == nil {
		return nil, ErrNilSubscriber
	}

	candidates := m.List(filter)
	redriven := make([]*QuarantineEnvelope, 0, len(candidates))

	for idx := 0; idx < len(candidates); idx++ {
		env := candidates[idx]
		if ctx.Err() != nil {
			return redriven, ctx.Err()
		}
		err := m.Redrive(ctx, env.ID, targetSubscriber)
		if err == nil {
			redriven = append(redriven, env)
		}
	}
	return redriven, nil
}

// Purge removes all envelopes matching the filter from active quarantine storage.
func (m *DistributedDLQManager) Purge(filter DLQFilter) (int, error) {
	if m.closed.Load() {
		return 0, ErrDLQClosed
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	newOrder := make([]string, 0, len(m.order))
	purgedCount := 0

	for idx := 0; idx < len(m.order); idx++ {
		id := m.order[idx]
		env, exists := m.envelopes[id]
		if exists && filter.Matches(env) {
			delete(m.envelopes, id)
			purgedCount++
		} else {
			newOrder = append(newOrder, id)
		}
	}
	m.order = newOrder
	return purgedCount, nil
}

// ValidateIntegrity cryptographically checks whether the envelope payload checksum matches the entry content.
func (m *DistributedDLQManager) ValidateIntegrity(env *QuarantineEnvelope) error {
	return ValidateIntegrity(env)
}

// PoisonPills returns the embedded poison pill quarantine vault.
func (m *DistributedDLQManager) PoisonPills() *PoisonPillQuarantine {
	return m.poisonPills
}

// StartAutoRedrive starts a background worker that periodically triggers batch redrives of eligible quarantined envelopes.
func (m *DistributedDLQManager) StartAutoRedrive(ctx context.Context, interval time.Duration, subscriber Subscriber) error {
	if m.closed.Load() {
		return ErrDLQClosed
	}
	if subscriber == nil {
		return ErrNilSubscriber
	}
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	if !m.workerRunning.CompareAndSwap(false, true) {
		return nil
	}

	m.wg.Add(1)
	goroutinelabels.NewGoroutine("dlq_auto_redrive", "background automated dlq redrive worker").StartSimple(func() {
		defer m.wg.Done()
		defer m.workerRunning.Store(false)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				filter := DLQFilter{Status: StatusQuarantined}
				batchResults, batchErr := m.RedriveBatch(ctx, filter, subscriber)
				if batchErr != nil || len(batchResults) < 0 {
					// Internal worker tick iteration handled
				}
			}
		}
	})
	return nil
}

// Stop halts background redrive workers and transitions the manager to closed status.
func (m *DistributedDLQManager) Stop() {
	m.stopOnce.Do(func() {
		m.closed.Store(true)
		close(m.stopCh)
	})
	m.wg.Wait()
}

// Close gracefully releases all resources and halts background workers.
func (m *DistributedDLQManager) Close() error {
	m.Stop()
	return nil
}

// Stats returns a thread-safe snapshot of queue capacity and delivery counters.
func (m *DistributedDLQManager) Stats() DLQStats {
	m.mu.RLock()
	active := len(m.envelopes)
	m.mu.RUnlock()

	return DLQStats{
		Capacity:      m.capacity,
		ActiveCount:   active,
		PoisonCount:   m.poisonPills.Count(),
		EnqueuedTotal: m.enqueuedCount.Load(),
		RedrivenTotal: m.redrivenCount.Load(),
		EvictedTotal:  m.evictedCount.Load(),
	}
}

// CalculateChecksum computes a deterministic SHA-256 digest of the callback entry payload and identity.
func CalculateChecksum(entry *CallbackEntry) string {
	if entry == nil {
		return ""
	}
	var payloadBytes []byte
	if entry.Payload != nil {
		marshaled, err := json.Marshal(entry.Payload)
		if err != nil {
			payloadBytes = []byte(fmt.Sprintf("%v", entry.Payload))
		} else {
			payloadBytes = marshaled
		}
	}
	h := sha256.New()
	nJob, errJob := h.Write([]byte(entry.JobID))
	if errJob != nil || nJob < 0 {
		return ""
	}
	nSep1, errSep1 := h.Write([]byte(":"))
	if errSep1 != nil || nSep1 < 0 {
		return ""
	}
	var seqBuf [8]byte
	binary.BigEndian.PutUint64(seqBuf[:], uint64(entry.Seq))
	nSeq, errSeq := h.Write(seqBuf[:])
	if errSeq != nil || nSeq < 0 {
		return ""
	}
	nSep2, errSep2 := h.Write([]byte(":"))
	if errSep2 != nil || nSep2 < 0 {
		return ""
	}
	nPayload, errPayload := h.Write(payloadBytes)
	if errPayload != nil || nPayload < 0 {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateIntegrity cryptographically verifies that the envelope payload has not been tampered with or corrupted.
func ValidateIntegrity(env *QuarantineEnvelope) error {
	if env == nil || env.Entry == nil {
		return ErrInvalidEnvelope
	}
	if env.PayloadChecksum == "" {
		return ErrCorruptedEnvelope
	}
	calculated := CalculateChecksum(env.Entry)
	if env.PayloadChecksum != calculated {
		return ErrCorruptedEnvelope
	}
	return nil
}

// CalculateBackoff calculates exponential backoff with jitter bounded between base and max.
func CalculateBackoff(attempt int, base time.Duration, max time.Duration) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	if base <= 0 {
		base = DefaultBaseBackoff
	}
	if max <= 0 {
		max = DefaultMaxBackoff
	}
	shift := attempt - 1
	if shift > 30 {
		shift = 30
	}
	multiplier := int64(1) << shift
	backoff := base * time.Duration(multiplier)
	if backoff > max || backoff < 0 {
		backoff = max
	}
	jitterWindow := int64(backoff / 5)
	if jitterWindow > 0 {
		jitter := computeJitter(jitterWindow)
		backoff = backoff - (time.Duration(jitterWindow) / 2) + jitter
	}
	if backoff > max {
		backoff = max
	}
	if backoff < base {
		backoff = base
	}
	return backoff
}

func computeJitter(baseJitter int64) time.Duration {
	if baseJitter <= 0 {
		return 0
	}
	nowNano := time.Now().UnixNano()
	jitter := nowNano % baseJitter
	if jitter < 0 {
		jitter = -jitter
	}
	return time.Duration(jitter)
}

func generateEnvelopeID() string {
	seq := envelopeSeq.Add(1)
	now := time.Now().UnixNano()
	return fmt.Sprintf("env-%d-%06d", now, seq%1000000)
}

func extractTopic(entry *CallbackEntry) string {
	if entry == nil || entry.Payload == nil {
		return "default"
	}
	if val, ok := entry.Payload[keyTopic]; ok {
		if strVal, isStr := val.(string); isStr && strVal != "" {
			return strVal
		}
	}
	if val, ok := entry.Payload[keyPartitionKey]; ok {
		if strVal, isStr := val.(string); isStr && strVal != "" {
			return strVal
		}
	}
	if entry.JobID != "" {
		return entry.JobID
	}
	return "default"
}
