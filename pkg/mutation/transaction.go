package mutation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TxnState represents the discrete lifecycle states of an ACID transaction.
type TxnState string

const (
	StateInitial  TxnState = "INITIAL"
	StateBegin    TxnState = "BEGIN"
	StateStage    TxnState = "STAGE"
	StatePreCheck TxnState = "PRE_CHECK"
	StateCommit   TxnState = "COMMIT"
	StateRollback TxnState = "ROLLBACK"
)

// IsolationMode defines the transactional isolation and execution semantics.
type IsolationMode string

const (
	IsolationAllOrNothing  IsolationMode = "all_or_nothing"
	IsolationPartialCommit IsolationMode = "partial_commit"
	IsolationDryRun        IsolationMode = "dry_run"

	// Alias names corresponding to grammar tokens
	IsolationStagedSnapshot IsolationMode = "STAGED_SNAPSHOT"
	IsolationSerializable  IsolationMode = "SERIALIZABLE"
	IsolationReadCommitted IsolationMode = "READ_COMMITTED"
)

// NormalizeIsolationMode maps any grammar alias to canonical isolation mode.
func NormalizeIsolationMode(mode IsolationMode) IsolationMode {
	switch mode {
	case IsolationPartialCommit:
		return IsolationPartialCommit
	case IsolationDryRun:
		return IsolationDryRun
	case IsolationAllOrNothing, IsolationStagedSnapshot, IsolationSerializable, IsolationReadCommitted, "":
		return IsolationAllOrNothing
	default:
		return IsolationAllOrNothing
	}
}

// ReceiptStatus defines the execution disposition of a mutation in a transaction.
type ReceiptStatus string

const (
	ReceiptStatusStaged          ReceiptStatus = "staged"
	ReceiptStatusCommitted       ReceiptStatus = "committed"
	ReceiptStatusQuarantined     ReceiptStatus = "quarantined"
	ReceiptStatusRolledBack      ReceiptStatus = "rolled_back"
	ReceiptStatusDryRunValidated ReceiptStatus = "dry_run_validated"
)

// DiagnosticReceipt records the execution outcome and diagnostic details of an individual mutation.
type DiagnosticReceipt struct {
	Index      int           `json:"index"`
	Action     Action        `json:"action"`
	TargetKind string        `json:"target_kind"`
	TargetID   string        `json:"target_id,omitempty"`
	Status     ReceiptStatus `json:"status"`
	Error      string        `json:"error,omitempty"`
}

var (
	ErrInvalidStateTransition = errors.New("invalid transaction state transition")
	ErrTransactionClosed      = errors.New("transaction is already completed (committed or rolled back)")
	ErrRollbackTriggered      = errors.New("transaction rolled back due to error")
	ErrValidationFailed       = errors.New("transaction pre-check validation failed")
)

// Transaction manages in-flight staged mutations, lifecycle state machine, and atomic rollback.
type Transaction struct {
	mu          sync.RWMutex
	id          string
	mode        IsolationMode
	state       TxnState
	staged      []Mutation
	receipts    []DiagnosticReceipt
	rollbackOps []func(ctx context.Context) error
}

// NewTransaction creates a new transaction with an unstarted INITIAL state.
func NewTransaction(id string, mode IsolationMode) *Transaction {
	return &Transaction{
		id:    id,
		mode:  NormalizeIsolationMode(mode),
		state: StateInitial,
	}
}

// ID returns the transaction identifier.
func (tx *Transaction) ID() string {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return tx.id
}

// Mode returns the transaction isolation mode.
func (tx *Transaction) Mode() IsolationMode {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return tx.mode
}

// State returns the current lifecycle state of the transaction.
func (tx *Transaction) State() TxnState {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return tx.state
}

// Receipts returns a copy of the diagnostic receipts.
func (tx *Transaction) Receipts() []DiagnosticReceipt {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	res := make([]DiagnosticReceipt, len(tx.receipts))
	copy(res, tx.receipts)
	return res
}

// StagedMutations returns a copy of the currently staged mutations.
func (tx *Transaction) StagedMutations() []Mutation {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	res := make([]Mutation, len(tx.staged))
	copy(res, tx.staged)
	return res
}

// Begin transitions the transaction from INITIAL to BEGIN.
func (tx *Transaction) Begin() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.state != StateInitial {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidStateTransition, tx.state, StateBegin)
	}
	tx.state = StateBegin
	return nil
}

// Stage adds a mutation to the transaction's private staged buffer.
func (tx *Transaction) Stage(mut Mutation) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.state != StateBegin && tx.state != StateStage && tx.state != StatePreCheck {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidStateTransition, tx.state, StateStage)
	}

	idx := len(tx.staged)
	tx.staged = append(tx.staged, mut)
	tx.receipts = append(tx.receipts, DiagnosticReceipt{
		Index:      idx,
		Action:     mut.Action,
		TargetKind: mut.TargetKind,
		TargetID:   mut.TargetID,
		Status:     ReceiptStatusStaged,
	})
	tx.state = StateStage
	return nil
}

// PreCheck transitions to PRE_CHECK and runs preflight verification on all staged mutations.
func (tx *Transaction) PreCheck(ctx context.Context, validator func(ctx context.Context, idx int, mut *Mutation) error) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.state != StateStage {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidStateTransition, tx.state, StatePreCheck)
	}
	tx.state = StatePreCheck

	if validator == nil {
		return nil
	}

	for i := range tx.staged {
		if err := validator(ctx, i, &tx.staged[i]); err != nil {
			if tx.mode == IsolationPartialCommit {
				tx.receipts[i].Status = ReceiptStatusQuarantined
				tx.receipts[i].Error = err.Error()
			} else {
				// all_or_nothing or dry_run failure
				tx.receipts[i].Error = err.Error()
				return fmt.Errorf("%w on item %d: %v", ErrValidationFailed, i, err)
			}
		}
	}

	return nil
}

// RegisterRollback registers an undo operation to execute if the transaction rolls back.
func (tx *Transaction) RegisterRollback(fn func(ctx context.Context) error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.rollbackOps = append(tx.rollbackOps, fn)
}

// Rollback triggers an atomic abort, executing any registered rollback handlers in reverse order.
func (tx *Transaction) Rollback(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.state == StateCommit {
		return fmt.Errorf("%w: cannot rollback an already committed transaction", ErrInvalidStateTransition)
	}
	if tx.state == StateRollback {
		return nil // idempotent
	}

	var rollbackErrs []error
	// Execute rollback handlers in reverse LIFO order
	for i := len(tx.rollbackOps) - 1; i >= 0; i-- {
		if err := tx.rollbackOps[i](ctx); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
	}

	for i := range tx.receipts {
		if tx.receipts[i].Status != ReceiptStatusQuarantined {
			tx.receipts[i].Status = ReceiptStatusRolledBack
		}
	}

	tx.state = StateRollback
	if len(rollbackErrs) > 0 {
		return fmt.Errorf("rollback encountered errors: %v", rollbackErrs)
	}
	return nil
}

// Commit atomically commits all staged mutations using the provided applier function.
func (tx *Transaction) Commit(ctx context.Context, applier func(ctx context.Context, idx int, mut *Mutation) (func(context.Context) error, error)) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.state != StatePreCheck {
		return fmt.Errorf("%w: cannot transition from %s to %s (pre-check required)", ErrInvalidStateTransition, tx.state, StateCommit)
	}

	if tx.mode == IsolationDryRun {
		for i := range tx.receipts {
			if tx.receipts[i].Status == ReceiptStatusStaged {
				tx.receipts[i].Status = ReceiptStatusDryRunValidated
			}
		}
		tx.state = StateCommit
		return nil
	}

	appliedRollbacks := make([]func(context.Context) error, 0, len(tx.staged))

	for i := range tx.staged {
		if tx.mode == IsolationPartialCommit && tx.receipts[i].Status == ReceiptStatusQuarantined {
			// Skip quarantined items in partial commit mode
			continue
		}

		undoFn, err := applier(ctx, i, &tx.staged[i])
		if err != nil {
			if tx.mode == IsolationAllOrNothing {
				// Record failure on record N
				tx.receipts[i].Error = err.Error()

				// Unwind all applied mutations in reverse order
				var unwindErrs []error
				for u := len(appliedRollbacks) - 1; u >= 0; u-- {
					if undoErr := appliedRollbacks[u](ctx); undoErr != nil {
						unwindErrs = append(unwindErrs, undoErr)
					}
				}
				// Also execute pre-existing rollback ops
				for u := len(tx.rollbackOps) - 1; u >= 0; u-- {
					if undoErr := tx.rollbackOps[u](ctx); undoErr != nil {
						unwindErrs = append(unwindErrs, undoErr)
					}
				}

				// Mark all receipts as rolled back
				for r := range tx.receipts {
					tx.receipts[r].Status = ReceiptStatusRolledBack
				}
				tx.state = StateRollback

				if len(unwindErrs) > 0 {
					return fmt.Errorf("%w: application error on record %d: %v; rollback errors: %v", ErrRollbackTriggered, i, err, unwindErrs)
				}
				return fmt.Errorf("%w: application error on record %d: %v", ErrRollbackTriggered, i, err)
			}

			// In partial_commit mode, quarantine this item
			tx.receipts[i].Status = ReceiptStatusQuarantined
			tx.receipts[i].Error = err.Error()
			continue
		}

		if undoFn != nil {
			appliedRollbacks = append(appliedRollbacks, undoFn)
		}
		tx.receipts[i].Status = ReceiptStatusCommitted
	}

	tx.state = StateCommit
	return nil
}

// TransactionEngine provides an isolated, thread-safe memory store to verify write isolation
// and atomic all-or-nothing execution without dirty read leakage.
type TransactionEngine struct {
	mu    sync.RWMutex
	store map[string]map[string]any
}

// NewTransactionEngine creates a new transaction engine.
func NewTransactionEngine() *TransactionEngine {
	return &TransactionEngine{
		store: make(map[string]map[string]any),
	}
}

// Get performs a thread-safe read from the committed store.
// Uncommitted mutations in any in-flight transaction are never visible here.
func (e *TransactionEngine) Get(ctx context.Context, id string) (map[string]any, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	item, exists := e.store[id]
	if !exists {
		return nil, false
	}
	// Return a deep copy to prevent data races
	copyMap := make(map[string]any, len(item))
	for k, v := range item {
		copyMap[k] = v
	}
	return copyMap, true
}

// Count returns the number of committed records in the store.
func (e *TransactionEngine) Count(ctx context.Context) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.store)
}

// Seed populates an initial node in the engine store (e.g. from an external resolver for updates).
func (e *TransactionEngine) Seed(id string, data map[string]any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.store == nil {
		e.store = make(map[string]map[string]any)
	}
	clone := make(map[string]any, len(data))
	for k, v := range data {
		clone[k] = v
	}
	e.store[id] = clone
}

// ExecuteTransaction executes a transaction against the engine with full ACID and isolation guarantees.
func (e *TransactionEngine) ExecuteTransaction(ctx context.Context, tx *Transaction) error {
	return e.ExecuteTransactionWithValidator(ctx, tx, nil)
}

// ExecuteTransactionWithValidator executes a transaction using an optional preflight validator.
func (e *TransactionEngine) ExecuteTransactionWithValidator(ctx context.Context, tx *Transaction, validator PreflightValidator) error {
	if tx.State() == StateInitial {
		if err := tx.Begin(); err != nil {
			return err
		}
	}

	// Pre-check phase
	if tx.State() == StateStage {
		if err := tx.PreCheck(ctx, func(ctx context.Context, idx int, mut *Mutation) error {
			if mut.TargetKind == "" {
				return errors.New("target_kind cannot be empty")
			}
			if validator != nil {
				receipt, valErr := validator.Validate(ctx, mut)
				if valErr != nil {
					return valErr
				}
				if !receipt.Valid {
					var msgs []string
					for _, v := range receipt.Violations {
						msgs = append(msgs, fmt.Sprintf("%s (%s: %s)", v.FieldPath, v.FailingConstraint, v.Actual))
					}
					return fmt.Errorf("%w: %s", ErrValidationFailed, strings.Join(msgs, "; "))
				}
			}
			return nil
		}); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
	}

	// Commit phase under write lock to guarantee atomicity and write isolation
	e.mu.Lock()
	defer e.mu.Unlock()

	return tx.Commit(ctx, func(ctx context.Context, idx int, mut *Mutation) (func(context.Context) error, error) {
		id := mut.TargetID
		if id == "" {
			return nil, errors.New("target_id is required for execution")
		}

		nowStr := time.Now().UTC().Format(time.RFC3339)
		actor := pkgctx.ActorIDForAttribution("")

		switch mut.Action {
		case ActionCreateNode:
			if fail, ok := mut.Fields["fail_trigger"].(bool); ok && fail {
				return nil, fmt.Errorf("injected constraint failure on node %s", id)
			}
			if _, exists := e.store[id]; exists {
				return nil, fmt.Errorf("node %s already exists", id)
			}
			nodeData := make(map[string]any)
			for k, v := range mut.Fields {
				nodeData[k] = v
			}
			nodeData["id"] = id
			nodeData["kind"] = mut.TargetKind
			if nodeData["created_at"] == nil {
				nodeData["created_at"] = nowStr
			}
			if nodeData["created_by"] == nil {
				nodeData["created_by"] = actor
			}
			if nodeData["updated_at"] == nil {
				nodeData["updated_at"] = nowStr
			}
			if nodeData["updated_by"] == nil {
				nodeData["updated_by"] = actor
			}
			e.store[id] = nodeData

			undo := func(ctx context.Context) error {
				delete(e.store, id)
				return nil
			}
			return undo, nil

		case ActionUpdateNode:
			prevData, exists := e.store[id]
			if !exists {
				return nil, fmt.Errorf("node %s not found for update", id)
			}
			// Save copy for undo
			savedCopy := make(map[string]any, len(prevData))
			for k, v := range prevData {
				savedCopy[k] = v
			}
			for k, v := range mut.Fields {
				prevData[k] = v
			}
			if !pkgctx.IsLifecycleBreakGlass(ctx) || prevData["updated_at"] == nil {
				prevData["updated_at"] = nowStr
			}
			if !pkgctx.IsLifecycleBreakGlass(ctx) || prevData["updated_by"] == nil {
				prevData["updated_by"] = actor
			}
			e.store[id] = prevData

			undo := func(ctx context.Context) error {
				e.store[id] = savedCopy
				return nil
			}
			return undo, nil

		case ActionRemoveEdge, ActionAddEdge:
			// Edge mutation simulation
			return func(ctx context.Context) error { return nil }, nil

		default:
			return nil, fmt.Errorf("unsupported action %s", mut.Action)
		}
	})
}
