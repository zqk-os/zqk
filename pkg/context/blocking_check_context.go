package context

import (
	"context"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
)

// BlockingCheckContext carries metadata for checking blocking issues before write operations
// This prevents write operations when there are critical violations that need to be resolved first
// It implements ProcessableContext to support async processing through the listener pipeline
type BlockingCheckContext struct {
	// ProjectRoot is the project root path (required for checking issues)
	ProjectRoot string

	// OperationKind is the kind of operation being performed (create, update, delete)
	OperationKind string

	// ObjectKind is the object kind being operated on
	ObjectKind string

	// ObjectID is the object ID being operated on (if applicable)
	ObjectID string

	// Bypass indicates whether to bypass the blocking check (for automated operations)
	Bypass bool

	// State tracks the processing state (implements ProcessableContext)
	state ContextState

	// ctx is the Go context for cancellation/timeout
	ctx context.Context

	// mu protects state access
	mu sync.RWMutex
}

// NewBlockingCheckContext creates a new blocking check context
func NewBlockingCheckContext(projectRoot, operationKind, objectKind, objectID string) *BlockingCheckContext {
	return &BlockingCheckContext{
		ProjectRoot:   projectRoot,
		OperationKind: operationKind,
		ObjectKind:    objectKind,
		ObjectID:      objectID,
		Bypass:        false,
		state:         StatePending,
		ctx:           NewSystemContext(),
	}
}

// WithBypass sets the bypass flag (for automated operations like audit_event, change_journal_entry)
func (b *BlockingCheckContext) WithBypass(bypass bool) *BlockingCheckContext {
	b.Bypass = bypass
	return b
}

// WithContext sets the Go context for cancellation/timeout
func (b *BlockingCheckContext) WithContext(ctx context.Context) *BlockingCheckContext {
	b.ctx = ctx
	return b
}

// GetState returns the current state (implements ProcessableContext)
func (b *BlockingCheckContext) GetState() ContextState {
	var state ContextState
	_ = concurrency.WithRLockCtx(
		&b.mu,
		NewSystemContext(),
		"blocking_check_context_get_state",
		func() error {
			state = b.state
			return nil
		},
	)
	return state
}

// SetState sets the state (implements ProcessableContext)
func (b *BlockingCheckContext) SetState(state ContextState) {
	_ = concurrency.WithLockCtx(
		&b.mu,
		NewSystemContext(),
		"blocking_check_context_set_state",
		func() error {
			b.state = state
			return nil
		},
	)
}

// GetContext returns the Go context (implements ProcessableContext)
func (b *BlockingCheckContext) GetContext() context.Context {
	return b.ctx
}

// BlockingIssue represents a Tier 1 (blocking) issue found during check
type BlockingIssue struct {
	ObjectID   string
	ObjectKind string
	Message    string
	Category   string
}

// BlockingCheckResult represents the result of a blocking check
type BlockingCheckResult struct {
	// HasBlockingIssues indicates whether blocking issues were found
	HasBlockingIssues bool

	// Issues contains the list of blocking issues found
	Issues []BlockingIssue

	// Error contains any error that occurred during the check
	Error error
}
