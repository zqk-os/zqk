package community

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TransactionPhase represents the operational lifecycle state of a transaction.
type TransactionPhase string

const (
	PhasePending    TransactionPhase = "pending"
	PhaseCommitted  TransactionPhase = "committed"
	PhaseRolledBack TransactionPhase = "rolled_back"
	PhaseFailed     TransactionPhase = "failed"
)

var (
	ErrTxAlreadyFinished = errors.New("transaction already committed or rolled back")
	ErrTxExecutionFailed = errors.New("transaction step execution failed; rollback initiated")
	ErrRollbackFailed    = errors.New("transaction rollback compensation encountered errors")
)

// CompensationAction represents a compensating rollback handler.
type CompensationAction func(ctx context.Context) error

// StepAction represents an executable transaction step.
type StepAction func(ctx context.Context) error

// TransactionStep holds execution and compensation handlers for an atomic operation.
type TransactionStep struct {
	ID          string
	Description string
	Execute     StepAction
	Compensate  CompensationAction
}

// TransactionRecord captures post-mortem audit telemetry for a completed transaction.
type TransactionRecord struct {
	TxID       string           `json:"tx_id"`
	ProviderID string           `json:"provider_id"`
	Phase      TransactionPhase `json:"phase"`
	StepCount  int              `json:"step_count"`
	Executed   int              `json:"executed"`
	Error      string           `json:"error,omitempty"`
	StartTime  time.Time        `json:"start_time"`
	Duration   time.Duration    `json:"duration"`
}

// Transaction manages the lifecycle of an atomic multi-step provider operation.
type Transaction struct {
	mu          sync.Mutex
	id          string
	providerID  string
	phase       TransactionPhase
	steps       []TransactionStep
	executedIdx int
	startTime   time.Time
	coordinator *TransactionCoordinator
}

// ID returns the unique transaction identifier.
func (tx *Transaction) ID() string {
	return tx.id
}

// ProviderID returns the bound provider ID.
func (tx *Transaction) ProviderID() string {
	return tx.providerID
}

// Phase returns the current operational phase.
func (tx *Transaction) Phase() TransactionPhase {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	return tx.phase
}

// AddStep appends an executable step and its compensating action to the transaction.
func (tx *Transaction) AddStep(id, description string, exec StepAction, comp CompensationAction) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.phase != PhasePending {
		return ErrTxAlreadyFinished
	}
	tx.steps = append(tx.steps, TransactionStep{
		ID:          id,
		Description: description,
		Execute:     exec,
		Compensate:  comp,
	})
	return nil
}

// ExecuteAndCommit executes each registered step in sequence, automatically rolling back on failure.
func (tx *Transaction) ExecuteAndCommit(ctx context.Context) error {
	tx.mu.Lock()
	if tx.phase != PhasePending {
		tx.mu.Unlock()
		return ErrTxAlreadyFinished
	}
	steps := make([]TransactionStep, len(tx.steps))
	copy(steps, tx.steps)
	tx.mu.Unlock()

	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			rbErr := tx.rollbackInternal(ctx, err)
			if rbErr != nil {
				return fmt.Errorf("%w: ctx err: %v, rollback err: %v", ErrRollbackFailed, err, rbErr)
			}
			return err
		}

		if step.Execute != nil {
			if err := step.Execute(ctx); err != nil {
				rbErr := tx.rollbackInternal(ctx, err)
				if rbErr != nil {
					return fmt.Errorf("%w: exec err: %v, rollback err: %v", ErrRollbackFailed, err, rbErr)
				}
				return fmt.Errorf("%w: %v", ErrTxExecutionFailed, err)
			}
		}

		tx.mu.Lock()
		tx.executedIdx = i + 1
		tx.mu.Unlock()
	}

	tx.mu.Lock()
	tx.phase = PhaseCommitted
	duration := time.Since(tx.startTime)
	record := TransactionRecord{
		TxID:       tx.id,
		ProviderID: tx.providerID,
		Phase:      PhaseCommitted,
		StepCount:  len(tx.steps),
		Executed:   tx.executedIdx,
		StartTime:  tx.startTime,
		Duration:   duration,
	}
	tx.mu.Unlock()

	tx.coordinator.recordTransaction(record)
	return nil
}

// Rollback manually rolls back all executed steps in reverse order (LIFO).
func (tx *Transaction) Rollback(ctx context.Context) error {
	return tx.rollbackInternal(ctx, errors.New("manual rollback requested"))
}

func (tx *Transaction) rollbackInternal(ctx context.Context, cause error) error {
	tx.mu.Lock()
	if tx.phase != PhasePending {
		tx.mu.Unlock()
		return ErrTxAlreadyFinished
	}

	executed := tx.executedIdx
	steps := make([]TransactionStep, executed)
	copy(steps, tx.steps[:executed])
	tx.phase = PhaseRolledBack
	duration := time.Since(tx.startTime)
	txID := tx.id
	providerID := tx.providerID
	tx.mu.Unlock()

	var rollbackErrs []error
	// Execute compensating actions in reverse order (LIFO)
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if step.Compensate != nil {
			if err := step.Compensate(ctx); err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("step %s compensation failed: %w", step.ID, err))
			}
		}
	}

	record := TransactionRecord{
		TxID:       txID,
		ProviderID: providerID,
		Phase:      PhaseRolledBack,
		StepCount:  len(steps),
		Executed:   executed,
		Error:      cause.Error(),
		StartTime:  tx.startTime,
		Duration:   duration,
	}

	var finalErr error
	if len(rollbackErrs) > 0 {
		record.Phase = PhaseFailed
		record.Error = fmt.Sprintf("cause: %v, rollback errors: %v", cause, errors.Join(rollbackErrs...))
		finalErr = errors.Join(rollbackErrs...)
	}

	tx.coordinator.recordTransaction(record)
	return finalErr
}

// TransactionCoordinator manages transactional journals across providers.
type TransactionCoordinator struct {
	mu        sync.RWMutex
	history   []TransactionRecord
	maxEvents int
	counter   uint64
}

// NewTransactionCoordinator initializes a thread-safe TransactionCoordinator.
func NewTransactionCoordinator(maxHistory int) *TransactionCoordinator {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	return &TransactionCoordinator{
		history:   make([]TransactionRecord, 0, maxHistory),
		maxEvents: maxHistory,
	}
}

// Begin begins a new transaction for a given provider.
func (c *TransactionCoordinator) Begin(providerID string) *Transaction {
	c.mu.Lock()
	c.counter++
	txID := fmt.Sprintf("tx-%d-%d", time.Now().UnixNano(), c.counter)
	c.mu.Unlock()

	return &Transaction{
		id:          txID,
		providerID:  providerID,
		phase:       PhasePending,
		steps:       make([]TransactionStep, 0, 4),
		startTime:   time.Now(),
		coordinator: c,
	}
}

// History returns a copy of recorded transaction telemetry records.
func (c *TransactionCoordinator) History() []TransactionRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]TransactionRecord, len(c.history))
	copy(result, c.history)
	return result
}

func (c *TransactionCoordinator) recordTransaction(record TransactionRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.history) >= c.maxEvents {
		c.history = c.history[1:]
	}
	c.history = append(c.history, record)
}

// SafeRunGoroutine executes an asynchronous worker under POL-CONCURRENCY guidelines.
func (c *TransactionCoordinator) SafeRunGoroutine(ctx context.Context, label string, fn func()) {
	goroutinelabels.NewGoroutine("community.tx."+label, "async transaction worker").StartSimple(fn)
}
