package context

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
)

// ValidationContext groups all validation-related state and configuration
// This follows the context object pattern to avoid scattered state/flags
type ValidationContext struct {
	// Project configuration
	ProjectRoot string

	// Validation execution configuration
	Workers     int           // Number of parallel validation workers
	MaxCacheAge time.Duration // Maximum age for cached validation results
	Timeout     time.Duration // Maximum time to wait for validation completion
	MaxRetries  int           // Maximum retry attempts for failed validations

	// Validation scope configuration
	EnabledTiers      []int    // Which tiers to validate (1=blocking, 2=warning, 3=info, 4=recommendation)
	EnabledKinds      []string // Which object kinds to validate (empty = all)
	EnabledCategories []string // Which issue categories to check (empty = all)
	SkipKinds         []string // Object kinds to skip

	// Cache configuration
	CacheOnly    bool // Only use cached results, don't perform new validation
	RefreshCache bool // Invalidate cache and force revalidation
	CacheEnabled bool // Whether to use cache at all

	// Priority configuration
	PriorityMode   string         // "tier" (default), "kind", "custom"
	CustomPriority map[string]int // Custom priority mapping (objectID or kind -> priority)

	// Progress reporting
	ProgressEnabled  bool          // Whether to show progress updates
	ProgressInterval time.Duration // How often to update progress

	// Validation filters
	FilterByTier     []int    // Only show issues of these tiers
	FilterByCategory []string // Only show issues of these categories
	FilterByKind     []string // Only show issues for these object kinds

	// Execution state
	Running  bool // Whether validation is currently running
	Paused   bool // Whether validation is paused
	Canceled bool // Whether validation was canceled

	// Internal state
	state ContextState
	ctx   context.Context
	mu    sync.RWMutex
}

// NewValidationContext creates a new validation context with defaults
func NewValidationContext(projectRoot string) *ValidationContext {
	return &ValidationContext{
		ProjectRoot:       projectRoot,
		Workers:           4,
		MaxCacheAge:       time.Hour,
		Timeout:           5 * time.Minute,
		MaxRetries:        3,
		EnabledTiers:      []int{1, 2, 3, 4}, // All tiers by default
		EnabledKinds:      []string{},        // All kinds by default
		EnabledCategories: []string{},        // All categories by default
		SkipKinds:         []string{},
		CacheOnly:         false,
		RefreshCache:      false,
		CacheEnabled:      true,
		PriorityMode:      "tier",
		CustomPriority:    make(map[string]int),
		ProgressEnabled:   true,
		ProgressInterval:  500 * time.Millisecond,
		FilterByTier:      []int{},
		FilterByCategory:  []string{},
		FilterByKind:      []string{},
		Running:           false,
		Paused:            false,
		Canceled:          false,
		state:             StatePending,
		ctx:               NewSystemContext(),
	}
}

// WithWorkers sets the number of validation workers
func (v *ValidationContext) WithWorkers(workers int) *ValidationContext {
	v.Workers = workers
	return v
}

// WithMaxCacheAge sets the maximum cache age
func (v *ValidationContext) WithMaxCacheAge(age time.Duration) *ValidationContext {
	v.MaxCacheAge = age
	return v
}

// WithTimeout sets the validation timeout
func (v *ValidationContext) WithTimeout(timeout time.Duration) *ValidationContext {
	v.Timeout = timeout
	return v
}

// WithEnabledTiers sets which tiers to validate
func (v *ValidationContext) WithEnabledTiers(tiers []int) *ValidationContext {
	v.EnabledTiers = tiers
	return v
}

// WithEnabledKinds sets which object kinds to validate
func (v *ValidationContext) WithEnabledKinds(kinds []string) *ValidationContext {
	v.EnabledKinds = kinds
	return v
}

// WithCacheOnly sets cache-only mode
func (v *ValidationContext) WithCacheOnly(cacheOnly bool) *ValidationContext {
	v.CacheOnly = cacheOnly
	return v
}

// WithRefreshCache sets refresh cache mode
func (v *ValidationContext) WithRefreshCache(refresh bool) *ValidationContext {
	v.RefreshCache = refresh
	return v
}

// WithContext sets the Go context for cancellation/timeout
func (v *ValidationContext) WithContext(ctx context.Context) *ValidationContext {
	v.ctx = ctx
	return v
}

// GetContext returns the Go context
func (v *ValidationContext) GetContext() context.Context {
	var ctx context.Context
	_ = concurrency.WithRLockCtx(
		&v.mu,
		NewSystemContext(),
		"validation_context_get_context",
		func() error {
			ctx = v.ctx
			return nil
		},
	)
	return ctx
}

// IsTierEnabled checks if a tier is enabled for validation
func (v *ValidationContext) IsTierEnabled(tier int) bool {
	if len(v.EnabledTiers) == 0 {
		return true // All tiers enabled if none specified
	}
	for _, t := range v.EnabledTiers {
		if t == tier {
			return true
		}
	}
	return false
}

// IsKindEnabled checks if an object kind is enabled for validation
func (v *ValidationContext) IsKindEnabled(kind string) bool {
	// Check skip list first
	for _, skip := range v.SkipKinds {
		if skip == kind {
			return false
		}
	}
	// If no enabled kinds specified, all are enabled
	if len(v.EnabledKinds) == 0 {
		return true
	}
	// Check if in enabled list
	for _, k := range v.EnabledKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// GetPriority returns the priority for an object
func (v *ValidationContext) GetPriority(objectID, objectKind string) int {
	// Check custom priority first
	if priority, ok := v.CustomPriority[objectID]; ok {
		return priority
	}
	if priority, ok := v.CustomPriority[objectKind]; ok {
		return priority
	}

	// Default: use tier-based priority
	// Objects with Tier 1 issues get priority 1, etc.
	// This is determined during validation, so default to tier 2 (warnings)
	return 2
}

// GetState returns the current state (implements ProcessableContext)
func (v *ValidationContext) GetState() ContextState {
	var state ContextState
	_ = concurrency.WithRLockCtx(
		&v.mu,
		NewSystemContext(),
		"validation_context_get_state",
		func() error {
			state = v.state
			return nil
		},
	)
	return state
}

// SetState sets the current state
func (v *ValidationContext) SetState(state ContextState) {
	_ = concurrency.WithLockCtx(
		&v.mu,
		NewSystemContext(),
		"validation_context_set_state",
		func() error {
			v.state = state
			return nil
		},
	)
}

// Validate performs validation on this context
func (v *ValidationContext) Validate() []ValidationError {
	var errors []ValidationError
	if v.ProjectRoot == emptyContextValue {
		errors = append(errors, ValidationError{
			Field:   "ProjectRoot",
			Message: "project root is required",
			Context: "ValidationContext",
		})
	}
	if v.Workers < 1 {
		errors = append(errors, ValidationError{
			Field:   "Workers",
			Message: "must have at least 1 worker",
			Context: "ValidationContext",
		})
	}
	if v.MaxCacheAge < 0 {
		errors = append(errors, ValidationError{
			Field:   "MaxCacheAge",
			Message: "max cache age cannot be negative",
			Context: "ValidationContext",
		})
	}
	return errors
}

// GetPrecedence returns the precedence level (implements ChainableContext)
func (v *ValidationContext) GetPrecedence() int {
	return PrecedenceProject // Project-level configuration
}

// SetPrecedence sets the precedence level
func (v *ValidationContext) SetPrecedence(precedence int) {
	// Precedence is fixed for validation context
}

// GetDepth returns the depth (implements ChainableContext)
func (v *ValidationContext) GetDepth() int {
	return DepthRoot
}

// SetDepth sets the depth
func (v *ValidationContext) SetDepth(depth int) {
	// Depth is fixed for validation context
}

// Merge merges this context into the target context (implements ChainableContext)
func (v *ValidationContext) Merge(target ChainableContext) ChainableContext {
	if target == nil {
		return v
	}
	// For validation context, we typically don't merge - use the most specific one
	// But if needed, we could merge settings
	return v
}
