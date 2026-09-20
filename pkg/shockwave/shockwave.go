package shockwave

import (
	"context"
	"fmt"
	"sync"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// TierMask represents bitmask flags for capability filtering or node targeting
type TierMask uint32

const (
	TierSecurity TierMask = 1 << iota
	TierSyntax
	TierSemantic
	TierExecution
)

// PropagationRules defines the rules for shockwave propagation
type PropagationRules struct {
	MaxDepth    int
	TargetTiers TierMask
	MaxWorkers  int // Bound the maximum concurrency
}

// Splitter divides a single payload into multiple discrete payloads
type Splitter interface {
	Split(ctx context.Context, payload interface{}) ([]interface{}, error)
}

// Duplicator duplicates a payload for broadcast purposes
type Duplicator interface {
	Duplicate(ctx context.Context, payload interface{}) ([]interface{}, error)
}

// Handler processes a single payload
type Handler interface {
	Handle(ctx context.Context, payload interface{}, targetTiers TierMask) (interface{}, error)
}

// Neuron represents an async, event-driven shockwave dispatcher.
// It uses a bounded goroutinelabels Pool to ensure strict thread-safety and limits.
type Neuron struct {
	mu          sync.RWMutex
	rules       PropagationRules
	splitters   []Splitter
	duplicators []Duplicator
	handlers    []Handler
	logger      logging.Logger

	// Event-driven async resources
	pool   *goroutinelabels.Pool
	outbox chan interface{}
}

// NewNeuron creates a new async event-driven neuron router safely bounding its concurrency
func NewNeuron(rules PropagationRules, logger logging.Logger) *Neuron {
	if logger == nil {
		logger = logging.GetLoggerFromProfile("system")
	}

	workerCount := rules.MaxWorkers
	if workerCount <= 0 {
		workerCount = 100 // Safe bounded default
	}

	// Utilize the existing common goroutinelabels Budget and Pool instead of reinventing unbounded goroutines
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: workerCount})
	pool := goroutinelabels.NewPool(budget, "shockwave-neuron", "event-driven dispatch", workerCount, workerCount*10)

	return &Neuron{
		rules:  rules,
		logger: logger,
		pool:   pool,
		outbox: make(chan interface{}, workerCount*10),
	}
}

// RegisterSplitter adds a splitter securely
func (r *Neuron) RegisterSplitter(s Splitter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.splitters = append(r.splitters, s)
}

// RegisterDuplicator adds a duplicator securely
func (r *Neuron) RegisterDuplicator(d Duplicator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.duplicators = append(r.duplicators, d)
}

// RegisterHandler adds a handler securely
func (r *Neuron) RegisterHandler(h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers = append(r.handlers, h)
}

// Start initializes the bounded worker pool. It is non-blocking.
func (r *Neuron) Start(ctx context.Context) {
	if r.logger != nil {
		logging.Fluent(r.logger).Info("Shockwave neuron started via goroutinelabels.Pool").Log()
	}
	r.pool.Start(ctx)
}

// Stop gracefully shuts down the bounded pool and closes outbox
func (r *Neuron) Stop() {
	if r.logger != nil {
		logging.Fluent(r.logger).Info("Shockwave neuron stopping").Log()
	}
	r.pool.Stop()
	close(r.outbox)
}

// Send non-blockingly submits a payload into the Neuron's worker pool.
func (r *Neuron) Send(ctx context.Context, payload interface{}) error {
	err := r.pool.SubmitNonBlocking(ctx, func(workerCtx context.Context) error {
		r.processPayload(workerCtx, payload)
		return nil
	})
	if err == goroutinelabels.ErrPoolFull {
		return fmt.Errorf("neuron inbox is completely full, shedding load")
	}
	return err
}

// Receive exposes the output channel for subscribing to results
func (r *Neuron) Receive() <-chan interface{} {
	return r.outbox
}

// processPayload safely processes payload without spawning unbounded resources
func (r *Neuron) processPayload(ctx context.Context, payload interface{}) {
	r.mu.RLock()
	splitters := r.splitters
	duplicators := r.duplicators
	handlers := r.handlers
	rules := r.rules
	r.mu.RUnlock()

	var mappedPayloads []interface{}
	var err error

	// Map Phase 1: Apply splitters or duplicators (if configured)
	if len(splitters) > 0 {
		for _, s := range splitters {
			res, e := s.Split(ctx, payload)
			if e != nil {
				err = e
				break
			}
			mappedPayloads = append(mappedPayloads, res...)
		}
	} else if len(duplicators) > 0 {
		for _, d := range duplicators {
			res, e := d.Duplicate(ctx, payload)
			if e != nil {
				err = e
				break
			}
			mappedPayloads = append(mappedPayloads, res...)
		}
	} else {
		mappedPayloads = []interface{}{payload}
	}

	if err != nil {
		if r.logger != nil {
			logging.Fluent(r.logger).Error("Shockwave map phase failed", err).Log()
		}
		return
	}

	// Map Phase 2 & Reduce: Process payloads with handlers
	for _, mp := range mappedPayloads {
		if len(handlers) > 0 {
			for _, h := range handlers {
				res, e := h.Handle(ctx, mp, rules.TargetTiers)
				if e != nil {
					if r.logger != nil {
						logging.Fluent(r.logger).Error("Shockwave handler failed", e).Log()
					}
					continue
				}
				// Route result out non-blockingly using default fallback
				select {
				case r.outbox <- res:
				case <-ctx.Done():
					return
				default:
					if r.logger != nil {
						logging.Fluent(r.logger).Warn("Shockwave outbox full, dropping mapped payload").Log()
					}
				}
			}
		} else {
			// No handlers, just forward
			select {
			case r.outbox <- mp:
			case <-ctx.Done():
				return
			default:
				if r.logger != nil {
					logging.Fluent(r.logger).Warn("Shockwave outbox full, dropping mapped payload").Log()
				}
			}
		}
	}
}
