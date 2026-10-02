package telemetry

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var errEmptyWorkerID = errors.New("worker id cannot be empty")

// Option configures a ThroughputAggregator.
type Option func(*ThroughputAggregator)

// WithWindow sets the sliding window duration.
func WithWindow(d time.Duration) Option {
	return func(a *ThroughputAggregator) {
		if d > 0 {
			a.window = d
		}
	}
}

// WithIdleThreshold sets the duration without activity after which a worker/system is considered idle.
func WithIdleThreshold(d time.Duration) Option {
	return func(a *ThroughputAggregator) {
		if d > 0 {
			a.idleThreshold = d
		}
	}
}

// WithClock overrides the clock function for deterministic testing.
func WithClock(fn func() time.Time) Option {
	return func(a *ThroughputAggregator) {
		if fn != nil {
			a.clock = fn
		}
	}
}

type sample struct {
	timestamp time.Time
	workerID  string
	tokens    int
}

// WorkerThroughput summarizes throughput metrics for a single worker.
type WorkerThroughput struct {
	WorkerID        string
	TotalTokens     int
	SampleCount     int
	TokensPerSecond float64
	Idle            bool
	IdleSince       *time.Time
}

// ThroughputSnapshot provides an aggregated snapshot of recent throughput activity.
type ThroughputSnapshot struct {
	SampleCount               int
	ActiveWorkers             int
	WindowTokens              int
	ThroughputTokensPerSecond float64
	Idle                      bool
	LastActivityAgo           time.Duration
}

// ThroughputAggregator collects, aggregates, and calculates real-time token throughput metrics.
type ThroughputAggregator struct {
	mu            sync.RWMutex
	window        time.Duration
	idleThreshold time.Duration
	clock         func() time.Time
	samples       []sample
	workerTokens  map[string]int
	workerSamples map[string]int
	lastActivity  map[string]time.Time
}

// NewThroughputAggregator creates a new ThroughputAggregator with default parameters.
func NewThroughputAggregator(opts ...Option) *ThroughputAggregator {
	a := &ThroughputAggregator{
		window:        time.Minute,
		idleThreshold: 30 * time.Second,
		clock:         time.Now,
		workerTokens:  make(map[string]int),
		workerSamples: make(map[string]int),
		lastActivity:  make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// RecordWorkerSample records tokens processed by a worker at the current timestamp.
func (a *ThroughputAggregator) RecordWorkerSample(ctx context.Context, workerID string, tokens int) error {
	if workerID == "" {
		return errEmptyWorkerID
	}

	now := a.clock()

	a.mu.Lock()
	defer a.mu.Unlock()

	a.samples = append(a.samples, sample{
		timestamp: now,
		workerID:  workerID,
		tokens:    tokens,
	})
	a.workerTokens[workerID] += tokens
	a.workerSamples[workerID]++
	a.lastActivity[workerID] = now

	return nil
}

func (a *ThroughputAggregator) withRLock(fn func()) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	fn()
}

// Workers returns all known worker IDs in deterministic sorted order.
func (a *ThroughputAggregator) Workers() []string {
	var workers []string
	a.withRLock(func() {
		workers = make([]string, 0, len(a.workerTokens))
		for w := range a.workerTokens {
			workers = append(workers, w)
		}
	})
	sort.Strings(workers)
	return workers
}

// Snapshot calculates windowed metrics across the aggregator.
func (a *ThroughputAggregator) Snapshot() ThroughputSnapshot {
	var snap ThroughputSnapshot
	a.withRLock(func() {
		now := a.clock()
		cutoff := now.Add(-a.window)

		windowTokens := 0
		activeSet := make(map[string]bool)
		var latestActivity time.Time

		for _, s := range a.samples {
			if s.timestamp.After(cutoff) || s.timestamp.Equal(cutoff) {
				windowTokens += s.tokens
			}
		}

		for w, t := range a.lastActivity {
			if t.After(cutoff) || t.Equal(cutoff) {
				activeSet[w] = true
			}
			if t.After(latestActivity) {
				latestActivity = t
			}
		}

		seconds := a.window.Seconds()
		rate := 0.0
		if seconds > 0 {
			rate = float64(windowTokens) / seconds
		}

		idle := false
		var lastAgo time.Duration
		if !latestActivity.IsZero() {
			lastAgo = now.Sub(latestActivity)
			if a.idleThreshold > 0 && lastAgo >= a.idleThreshold {
				idle = true
			}
		} else {
			idle = true
		}

		snap = ThroughputSnapshot{
			SampleCount:               len(a.samples),
			ActiveWorkers:             len(activeSet),
			WindowTokens:              windowTokens,
			ThroughputTokensPerSecond: rate,
			Idle:                      idle,
			LastActivityAgo:           lastAgo,
		}
	})
	return snap
}

// WorkerDetails returns individual worker stats sorted by total tokens descending.
func (a *ThroughputAggregator) WorkerDetails() []WorkerThroughput {
	var details []WorkerThroughput
	a.withRLock(func() {
		now := a.clock()
		details = make([]WorkerThroughput, 0, len(a.workerTokens))

		for w, total := range a.workerTokens {
			last := a.lastActivity[w]
			isIdle := false
			var idleSince *time.Time
			if a.idleThreshold > 0 && now.Sub(last) >= a.idleThreshold {
				isIdle = true
				t := last.Add(a.idleThreshold)
				idleSince = &t
			}

			details = append(details, WorkerThroughput{
				WorkerID:    w,
				TotalTokens: total,
				SampleCount: a.workerSamples[w],
				Idle:        isIdle,
				IdleSince:   idleSince,
			})
		}

		sort.Slice(details, func(i, j int) bool {
			if details[i].TotalTokens == details[j].TotalTokens {
				return details[i].WorkerID < details[j].WorkerID
			}
			return details[i].TotalTokens > details[j].TotalTokens
		})
	})

	return details
}

// Reset clears all recorded samples and worker records.
func (a *ThroughputAggregator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.samples = nil
	a.workerTokens = make(map[string]int)
	a.workerSamples = make(map[string]int)
	a.lastActivity = make(map[string]time.Time)
}
