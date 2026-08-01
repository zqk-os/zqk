package objects

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// LifecycleIOConfig holds configuration for lifecycle I/O operations
// Matches RetryConfig pattern from pkg/storage/operation_helper.go
// Defined locally to avoid import cycle (pkg/storage imports pkg/objects)
type LifecycleIOConfig struct {
	// Retry configuration (matches RetryConfig from operation_helper.go)
	RetryMaxAttempts   int
	RetryInitialDelay  time.Duration
	RetryMaxDelay      time.Duration
	RetryBackoffFactor float64

	// I/O timeout
	IOTimeout time.Duration
}

// DefaultLifecycleIOConfig returns default configuration for lifecycle I/O operations
// Values match defaults from pkg/storage/operation_helper.go RetryConfig
func DefaultLifecycleIOConfig() *LifecycleIOConfig {
	return &LifecycleIOConfig{
		RetryMaxAttempts:   3,
		RetryInitialDelay:  100 * time.Millisecond,
		RetryMaxDelay:      5 * time.Second,
		RetryBackoffFactor: 2.0,
		IOTimeout:          5 * time.Second,
	}
}

// Lifecycle represents a lifecycle definition loaded from YAML.
// Location is the path reference (prefix: or abs:) for the backing file so it can be re-used and resolved where necessary.
type Lifecycle struct {
	ObjectType      string                `yaml:"object_type"`
	Extends         string                `yaml:"extends,omitempty"`        // Parent lifecycle to extend (e.g., "base_lifecycle")
	StatusMapping   map[string]string     `yaml:"status_mapping,omitempty"` // Maps internal (base) statuses to external (this lifecycle) statuses
	Statuses        []Status              `yaml:"statuses"`
	Transitions     []Transition          `yaml:"transitions"`
	PercentComplete PercentCompleteConfig `yaml:"percent_complete"`
	// Location is set by the loader to a path reference (prefix:relPath or abs:absolutePath) for the backing YAML file.
	Location string `yaml:"-"`
}

// Status represents a lifecycle status
type Status struct {
	Value         string   `yaml:"value"`
	Display       string   `yaml:"display"`
	Origin        bool     `yaml:"origin,omitempty"`
	Terminal      bool     `yaml:"terminal,omitempty"`
	Preliminary   bool     `yaml:"preliminary,omitempty"`
	Archive       bool     `yaml:"archive,omitempty"`
	System        bool     `yaml:"system,omitempty"`
	Preconditions []string `yaml:"preconditions,omitempty"`
	Description   string   `yaml:"description,omitempty"`
}

// Transition represents a valid state transition
type Transition struct {
	From          string   `yaml:"from"`
	To            string   `yaml:"to"`
	Description   string   `yaml:"description"`
	Manual        bool     `yaml:"manual"`
	Auto          bool     `yaml:"auto"`
	Preconditions []string `yaml:"preconditions,omitempty"`
}

// PercentCompleteConfig defines how to calculate percent complete
type PercentCompleteConfig struct {
	Method          string         `yaml:"method"`
	DefaultByStatus map[string]any `yaml:"default_by_status,omitempty"` // Can be int or string (e.g., "calculated")
	MilestoneBased  map[string]any `yaml:"milestone_based,omitempty"`
}

// cachedLifecycle stores a lifecycle with its file modification time for staleness detection
type cachedLifecycle struct {
	lifecycle *Lifecycle
	mtime     time.Time // File modification time when cached
}

// LifecycleLoader loads lifecycle definitions from YAML files.
// Optional EnsureReady(ctx) uses the component loader pattern (pkg/loader) to warm base lifecycle once with timeout.
type LifecycleLoader struct {
	lifecyclesDir   atomic.Value // string
	cache           sync.Map     // kind -> *cachedLifecycle
	readyRunner     *loader.Runner
	readyRunnerOnce sync.Once
}

// NewLifecycleLoader creates a new lifecycle loader
func NewLifecycleLoader(lifecyclesDir string) *LifecycleLoader {
	ll := &LifecycleLoader{}
	if lifecyclesDir != emptyValue {
		ll.lifecyclesDir.Store(lifecyclesDir)
	}
	return ll
}

// getLifecyclesDir returns the lazily-loaded lifecycles directory.
func (ll *LifecycleLoader) getLifecyclesDir() string {
	v := ll.lifecyclesDir.Load()
	if v != nil {
		return v.(string)
	}
	dir := findLifecyclesDir()
	ll.lifecyclesDir.Store(dir)
	return dir
}

// getReadyRunner returns the shared loader.Runner for "ensure ready" (lazily created).
func (ll *LifecycleLoader) getReadyRunner() *loader.Runner {
	ll.readyRunnerOnce.Do(func() {
		ll.readyRunner = loader.NewRunner(KindLifecycle, func(ctx context.Context) error {
			return ll.doEnsureReady(ctx)
		})
	})
	return ll.readyRunner
}

// EnsureReady ensures the lifecycle loader is ready: lifecycles dir is set and base lifecycle is warmed.
// Uses the component loader pattern (pkg/loader) so concurrent callers wait with timeout.
// Optional: call before heavy use; per-kind loading remains unchanged.
func (ll *LifecycleLoader) EnsureReady(ctx context.Context) error {
	return ll.getReadyRunner().Load(ctx)
}

// doEnsureReady runs once per loader; sets lifecyclesDir if empty and preloads base_object lifecycle.
func (ll *LifecycleLoader) doEnsureReady(ctx context.Context) error { //nolint:unparam // ctx for LoadFn signature
	ll.getLifecyclesDir()
	_, _ = ll.LoadLifecycle(KindBaseObject)
	return nil
}

// LoadLifecycle loads a lifecycle definition for a given object kind
// If no specific lifecycle exists for the kind, falls back to base_object_lifecycle.yaml
func (ll *LifecycleLoader) LoadLifecycle(kind string) (*Lifecycle, error) {
	currentDir := ll.getLifecyclesDir()

	lifecycleFile := fmt.Sprintf("%s_lifecycle.yaml", kind)
	lifecyclePath := filepath.Join(currentDir, lifecycleFile)

	if cachedVal, ok := ll.cache.Load(kind); ok {
		cached := cachedVal.(*cachedLifecycle)
		stat, err := ll.statFileWithTimeout(pkgctx.NewSystemContext(), lifecyclePath, 2*time.Second)
		if err == nil && stat != nil {
			if stat.ModTime().Equal(cached.mtime) || stat.ModTime().Before(cached.mtime) {
				return cached.lifecycle, nil
			}
		}
	}

	ctx := pkgctx.NewSystemContext()
	data, err := ll.readFileWithTimeout(ctx, lifecyclePath, 5*time.Second)
	if err != nil {
		if os.IsNotExist(err) {
			baseLifecyclePath := filepath.Join(currentDir, "base_object_lifecycle.yaml")
			baseData, baseErr := ll.readFileWithTimeout(ctx, baseLifecyclePath, 5*time.Second)
			if baseErr == nil {
				data = baseData
				lifecyclePath = baseLifecyclePath
			} else {
				return nil, errfmt.Errorf("failed to read lifecycle file %s: %w (fallback also failed: %v)", lifecyclePath, err, baseErr)
			}
		} else {
			return nil, errfmt.Errorf("failed to read lifecycle file %s: %w", lifecyclePath, err)
		}
	}

	var lifecycle Lifecycle
	if err := yaml.Unmarshal(data, &lifecycle); err != nil {
		return nil, errfmt.Errorf("failed to parse lifecycle file %s: %w", lifecyclePath, err)
	}

	if lifecycle.Extends != emptyValue {
		parentLifecycle, err := ll.loadLifecycleWithExtends(lifecycle.Extends, make(map[string]bool))
		if err != nil {
			return nil, errfmt.Errorf("failed to load parent lifecycle %s: %w", lifecycle.Extends, err)
		}
		lifecycle = ll.mergeLifecycles(parentLifecycle, &lifecycle)
	}

	lifecycle.Location = pathRefForFile(lifecyclePath)

	var mtime time.Time
	stat, err := ll.statFileWithTimeout(ctx, lifecyclePath, 2*time.Second)
	if err == nil && stat != nil {
		mtime = stat.ModTime()
	} else {
		mtime = time.Now()
	}

	if cachedAfterVal, ok := ll.cache.Load(kind); ok {
		cachedAfter := cachedAfterVal.(*cachedLifecycle)
		statAfter, statErr := ll.statFileWithTimeout(pkgctx.NewSystemContext(), lifecyclePath, 2*time.Second)
		if statErr == nil && statAfter != nil && !statAfter.ModTime().After(cachedAfter.mtime) {
			return cachedAfter.lifecycle, nil
		}
	}

	newCached := &cachedLifecycle{lifecycle: &lifecycle, mtime: mtime}
	ll.cache.Store(kind, newCached)
	return &lifecycle, nil
}

var (
	globalLifecycleLoader *LifecycleLoader
	lifecycleLoaderOnce   sync.Once
)

// GetGlobalLifecycleLoader returns the global lifecycle loader instance
// This ensures caches are shared across all validation operations (sync and async)
// The lifecycle directory is found lazily on first use to avoid blocking on filesystem I/O during initialization
func GetGlobalLifecycleLoader() *LifecycleLoader {
	lifecycleLoaderOnce.Do(func() {
		globalLifecycleLoader = &LifecycleLoader{}
	})
	return globalLifecycleLoader
}

// ClearCache clears the lifecycle cache, forcing reload of all lifecycles on next access
// This is useful when lifecycle definitions are updated and you want to ensure fresh validation
// Note: This uses a write lock and blocks all readers. For better performance,
// use InvalidateLifecycle() to invalidate specific entries.
func (ll *LifecycleLoader) ClearCache() {
	ll.cache.Clear()
}

// InvalidateLifecycle invalidates a specific lifecycle entry by kind
// This is non-blocking for readers (uses write lock but only for one entry)
// Use this instead of ClearCache() when you know which lifecycle changed
func (ll *LifecycleLoader) InvalidateLifecycle(kind string) {
	ll.cache.Delete(kind)
}

// IsValidStatus checks if a status is valid for the given kind.
// Accepts common variants (e.g. "completed" -> "complete", "archive" -> "archived")
// via ApplyAliasesForStatus so aliases are applied only when the preferred value is in this kind's lifecycle.
func (ll *LifecycleLoader) IsValidStatus(kind, status string) (bool, error) {
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return false, err
	}
	validSet := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		validSet[s.Value] = true
	}
	canonical := ApplyAliasesForStatus(status, validSet)
	for _, s := range lifecycle.Statuses {
		if s.Value == canonical {
			return true, nil
		}
	}
	return false, nil
}

// GetAllowedStatuses returns the list of valid status values for the given kind from its lifecycle.
// Returns nil, nil when the kind has no lifecycle (caller can fall back to spec enum).
func (ll *LifecycleLoader) GetAllowedStatuses(kind string) ([]string, error) {
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return nil, err
	}
	if len(lifecycle.Statuses) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(lifecycle.Statuses))
	for _, s := range lifecycle.Statuses {
		out = append(out, s.Value)
	}
	return out, nil
}

// IsValidTransition checks if a transition from one state to another is valid.
// Normalizes from/to with ApplyAliasesForStatus (kind-aware) before validation.
func (ll *LifecycleLoader) IsValidTransition(kind, from, to string) (bool, error) {
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return false, err
	}
	validSet := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		validSet[s.Value] = true
	}
	fromCanonical := ApplyAliasesForStatus(from, validSet)
	toCanonical := ApplyAliasesForStatus(to, validSet)

	// Check if both states are valid
	fromValid, err := ll.IsValidStatus(kind, fromCanonical)
	if err != nil {
		return false, err
	}
	if !fromValid {
		return false, errfmt.Errorf("invalid source status: %s", from)
	}

	toValid, err := ll.IsValidStatus(kind, toCanonical)
	if err != nil {
		return false, err
	}
	if !toValid {
		return false, errfmt.Errorf("invalid target status: %s", to)
	}

	// Check if transition exists
	for _, transition := range lifecycle.Transitions {
		if (transition.From == fromCanonical || transition.From == "*") && transition.To == toCanonical {
			return true, nil
		}
	}

	return false, nil
}

// NormalizeStatusForKind returns the preferred status for this kind when the given
// status is a registered alias and the preferred value is in the kind's lifecycle.
// Use when persisting (e.g. Create/Update) so stored values are canonical. Returns
// the original status if lifecycle load fails or no alias applies.
func (ll *LifecycleLoader) NormalizeStatusForKind(kind, status string) (string, error) {
	if status == emptyValue {
		return status, nil
	}
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return status, err
	}
	validSet := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		validSet[s.Value] = true
	}
	return ApplyAliasesForStatus(status, validSet), nil
}

// GetTransitionPreconditions returns the preconditions for a transition
func (ll *LifecycleLoader) GetTransitionPreconditions(kind, from, to string) ([]string, error) {
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return nil, err
	}

	for _, transition := range lifecycle.Transitions {
		if transition.From == from && transition.To == to {
			return transition.Preconditions, nil
		}
	}

	return nil, errfmt.Errorf("transition not found: %s -> %s", from, to)
}

// GetStatusPreconditions returns the preconditions for a status
func (ll *LifecycleLoader) GetStatusPreconditions(kind, status string) ([]string, error) {
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return nil, err
	}

	for _, s := range lifecycle.Statuses {
		if s.Value == status {
			return s.Preconditions, nil
		}
	}

	return nil, errfmt.Errorf("status not found: %s", status)
}

// GetOriginStatus returns the origin status for a kind
func (ll *LifecycleLoader) GetOriginStatus(kind string) (string, error) {
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return "", err
	}

	for _, s := range lifecycle.Statuses {
		if s.Origin {
			return s.Value, nil
		}
	}

	return "", errfmt.Errorf("no origin status found for kind: %s", kind)
}

// IsTerminalStatusForKind reports whether status is terminal for kind (lifecycle YAML terminal: true).
// Normalizes status with the kind's valid status set (aliases). Unknown or empty status is non-terminal.
func (ll *LifecycleLoader) IsTerminalStatusForKind(kind, status string) (bool, error) {
	if status == emptyValue {
		return false, nil
	}
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return false, err
	}
	validSet := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		validSet[s.Value] = true
	}
	canonical := ApplyAliasesForStatus(status, validSet)
	for _, s := range lifecycle.Statuses {
		if s.Value == canonical {
			return s.Terminal, nil
		}
	}
	return false, nil
}

// IsPreliminaryStatusForKind reports whether status is preliminary/draft for kind.
func (ll *LifecycleLoader) IsPreliminaryStatusForKind(kind, status string) (bool, error) {
	if status == emptyValue {
		return false, nil
	}
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return false, err
	}
	validSet := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		validSet[s.Value] = true
	}
	canonical := ApplyAliasesForStatus(status, validSet)
	for _, s := range lifecycle.Statuses {
		if s.Value == canonical {
			if s.Preliminary {
				return true, nil
			}
			if s.Origin {
				statusLower := strings.ToLower(s.Value)
				return statusLower != "active" && statusLower != "approved" && statusLower != "in_progress" && statusLower != "implemented", nil
			}
			return false, nil
		}
	}
	return false, nil
}

// pathRefForFile returns a path reference (prefix:relPath or abs:absolutePath) for the given file path.
// Enables spec-like objects backed by YAML to reference their location for reuse and resolution.
func pathRefForFile(filePath string) string {
	if filePath == emptyValue {
		return ""
	}
	if filepath.IsAbs(filePath) {
		return paths.PathSchemeAbs + filePath
	}
	return paths.PathRefFromRelPath(filePath)
}

// findLifecyclesDir finds the lifecycles directory
// Uses async I/O with timeout and retry logic to prevent blocking
// Per concurrency-patterns-v1.0.md and established RetryConfig pattern
func findLifecyclesDir() string {
	lc := paths.ProcessInternalLifecyclesDir
	// Try common locations
	possibleDirs := []string{
		lc,
		filepath.Join("..", "..", lc),
		filepath.Join("..", "..", "..", lc),
	}

	ctx := pkgctx.NewSystemContext()
	// Use established RetryConfig pattern (matches pkg/storage/operation_helper.go)
	// Fewer attempts for directory discovery (faster failure)
	config := &LifecycleIOConfig{
		RetryMaxAttempts:   2,
		RetryInitialDelay:  50 * time.Millisecond,
		RetryMaxDelay:      200 * time.Millisecond,
		RetryBackoffFactor: 2.0,
		IOTimeout:          2 * time.Second,
	}

	for _, dir := range possibleDirs {
		var lastErr error
		delay := config.RetryInitialDelay

		// Execute Stat with retry logic (per architecture requirements)
	retryLoop:
		for attempt := 0; attempt < config.RetryMaxAttempts; attempt++ {
			// Check context cancellation
			if ctx.Err() != nil {
				break retryLoop // Continue to next directory
			}

			// Non-blocking Stat with timeout (per IO + Async On-Demand pattern)
			type statResult struct {
				info os.FileInfo
				err  error
			}
			resultChan := make(chan statResult, 1)

			goroutinelabels.NewGoroutine("lifecycle_loader_find_dir", fmt.Sprintf("checking directory %s (attempt %d/%d)", dir, attempt+1, config.RetryMaxAttempts)).
				WithContext(ctx).
				StartSimple(func() {
					info, err := os.Stat(dir)
					resultChan <- statResult{info: info, err: err}
				})

			// Wait for result with timeout
			select {
			case result := <-resultChan:
				if result.err == nil && result.info != nil && result.info.IsDir() {
					// Success - found directory
					return dir
				}
				lastErr = result.err
				// File not found is expected for wrong directories - not retryable
				if os.IsNotExist(result.err) {
					break retryLoop // Continue to next directory
				}
				// Check if error is retryable
				if !isRetryableFileError(result.err) {
					break retryLoop // Continue to next directory
				}
			case <-time.After(config.IOTimeout):
				// Stat timeout - retryable error
				lastErr = errfmt.Errorf("stat operation timed out after %v", config.IOTimeout)
			case <-ctx.Done():
				// Context cancelled - continue to next directory
				break retryLoop
			}

			// If we found the directory, break out of retry loop
			if lastErr == nil {
				break retryLoop
			}

			// Last attempt, don't wait
			if attempt == config.RetryMaxAttempts-1 {
				break retryLoop
			}

			// Wait before retry with exponential backoff (per architecture pattern)
			select {
			case <-ctx.Done():
				break retryLoop
			case <-time.After(delay):
				// Continue to next attempt
			}

			// Exponential backoff
			delay = time.Duration(float64(delay) * config.RetryBackoffFactor)
			if delay > config.RetryMaxDelay {
				delay = config.RetryMaxDelay
			}
		}
	}

	return paths.ProcessInternalLifecyclesDir // Default
}

// loadLifecycleWithExtends loads a lifecycle and resolves its inheritance chain
// visited tracks visited lifecycles to detect circular dependencies
func (ll *LifecycleLoader) loadLifecycleWithExtends(lifecycleName string, visited map[string]bool) (*Lifecycle, error) {
	// Check for circular dependencies
	if visited[lifecycleName] {
		return nil, errfmt.Errorf("circular dependency detected in lifecycle inheritance: %s", lifecycleName)
	}
	visited[lifecycleName] = true

	// Resolve lifecycle filename
	// Supports multiple naming patterns:
	// - "base_lifecycle" -> "base_object_lifecycle.yaml" (maps to actual file name)
	// - "parent_lifecycle" -> "parent_lifecycle.yaml"
	// - "parent" -> "parent_lifecycle.yaml"
	var lifecycleFile string
	if lifecycleName == "base_lifecycle" {
		lifecycleFile = "base_object_lifecycle.yaml"
	} else if strings.HasSuffix(lifecycleName, "_lifecycle") {
		lifecycleFile = lifecycleName + ".yaml"
	} else {
		lifecycleFile = lifecycleName + "_lifecycle.yaml"
	}

	lifecyclePath := filepath.Join(ll.getLifecyclesDir(), lifecycleFile)
	ctx := pkgctx.NewSystemContext()
	data, err := ll.readFileWithTimeout(ctx, lifecyclePath, 5*time.Second)
	if err != nil {
		return nil, errfmt.Errorf("failed to read lifecycle file %s: %w", lifecyclePath, err)
	}

	var lifecycle Lifecycle
	if err := yaml.Unmarshal(data, &lifecycle); err != nil {
		return nil, errfmt.Errorf("failed to parse lifecycle file %s: %w", lifecyclePath, err)
	}

	// Recursively load parent if this lifecycle extends another
	if lifecycle.Extends != emptyValue {
		parentLifecycle, err := ll.loadLifecycleWithExtends(lifecycle.Extends, visited)
		if err != nil {
			return nil, errfmt.Errorf("failed to load parent lifecycle %s: %w", lifecycle.Extends, err)
		}
		// Merge parent into child (child overrides parent)
		lifecycle = ll.mergeLifecycles(parentLifecycle, &lifecycle)
	}

	return &lifecycle, nil
}

// mergeLifecycles merges a parent lifecycle into a child lifecycle
// Child statuses and transitions override parent ones
// Status mapping is applied to map internal (parent) statuses to external (child) statuses
func (ll *LifecycleLoader) mergeLifecycles(parent, child *Lifecycle) Lifecycle {
	merged := Lifecycle{
		ObjectType:      child.ObjectType,
		StatusMapping:   make(map[string]string),
		Statuses:        make([]Status, 0),
		Transitions:     make([]Transition, 0),
		PercentComplete: child.PercentComplete,
	}

	// Copy child's status mapping, or create identity mapping if none exists
	if len(child.StatusMapping) > 0 {
		for k, v := range child.StatusMapping {
			merged.StatusMapping[k] = v
		}
	} else {
		// Create identity mapping for all parent statuses
		for _, status := range parent.Statuses {
			merged.StatusMapping[status.Value] = status.Value
		}
	}

	// Start with parent statuses, then add/override with child statuses
	statusMap := make(map[string]Status)
	for _, status := range parent.Statuses {
		// Map internal status to external if mapping exists
		externalValue := merged.StatusMapping[status.Value]
		if externalValue == emptyValue {
			externalValue = status.Value
		}
		status.Value = externalValue
		statusMap[externalValue] = status
	}

	// Override/add child statuses
	for _, status := range child.Statuses {
		statusMap[status.Value] = status
	}

	// Convert map to slice
	for _, status := range statusMap {
		merged.Statuses = append(merged.Statuses, status)
	}

	// Start with parent transitions, then add/override with child transitions
	transitionMap := make(map[string]Transition) // key: "from->to"
	for _, transition := range parent.Transitions {
		// Map internal statuses to external using status mapping
		from := merged.StatusMapping[transition.From]
		if from == emptyValue {
			from = transition.From
		}
		to := merged.StatusMapping[transition.To]
		if to == emptyValue {
			to = transition.To
		}
		transition.From = from
		transition.To = to
		key := fmt.Sprintf("%s->%s", from, to)
		transitionMap[key] = transition
	}

	// Override/add child transitions
	for _, transition := range child.Transitions {
		key := fmt.Sprintf("%s->%s", transition.From, transition.To)
		transitionMap[key] = transition
	}

	// Convert map to slice
	for _, transition := range transitionMap {
		merged.Transitions = append(merged.Transitions, transition)
	}

	// Merge percent_complete defaults (child overrides parent)
	if merged.PercentComplete.DefaultByStatus == nil {
		merged.PercentComplete.DefaultByStatus = make(map[string]any)
	}
	if parent.PercentComplete.DefaultByStatus != nil {
		for k, v := range parent.PercentComplete.DefaultByStatus {
			if _, exists := merged.PercentComplete.DefaultByStatus[k]; !exists {
				merged.PercentComplete.DefaultByStatus[k] = v
			}
		}
	}

	return merged
}

// readFileWithTimeout reads a file asynchronously with a timeout and retry logic
// Follows async I/O best practices: non-blocking goroutine + timeout + retry with exponential backoff
// Per concurrency-patterns-v1.0.md and established RetryConfig pattern from pkg/storage/operation_helper.go
func (ll *LifecycleLoader) readFileWithTimeout(ctx context.Context, filePath string, timeout time.Duration) ([]byte, error) {
	// Use established RetryConfig pattern (matches pkg/storage/operation_helper.go)
	config := DefaultLifecycleIOConfig()
	config.IOTimeout = timeout // Use provided timeout

	var lastErr error
	delay := config.RetryInitialDelay

	// Execute ReadFile with retry logic (per architecture requirements)
	for attempt := 0; attempt < config.RetryMaxAttempts; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return nil, errfmt.Newf("read operation cancelled").Wrap(ctx.Err())
		}

		// Non-blocking ReadFile with timeout (per IO + Async On-Demand pattern)
		type readResult struct {
			data []byte
			err  error
		}
		resultChan := make(chan readResult, 1)

		// Execute ReadFile in goroutine to prevent blocking
		goroutinelabels.NewGoroutine("lifecycle_loader_read_file", fmt.Sprintf("reading lifecycle file %s with timeout (attempt %d/%d)", filepath.Base(filePath), attempt+1, config.RetryMaxAttempts)).
			WithContext(ctx).
			StartSimple(func() {
				data, err := os.ReadFile(filePath)
				resultChan <- readResult{data: data, err: err}
			})

		// Wait for result with timeout (non-blocking IO pattern)
		select {
		case result := <-resultChan:
			if result.err == nil {
				// Success - return immediately
				return result.data, nil
			}
			lastErr = result.err
			// Check if error is retryable (timeout errors are retryable)
			if !isRetryableFileError(result.err) {
				return nil, result.err
			}
		case <-time.After(config.IOTimeout):
			// ReadFile timeout - retryable error
			lastErr = errfmt.Errorf("read operation timed out after %v", config.IOTimeout)
		case <-ctx.Done():
			// Context cancelled - non-retryable
			return nil, errfmt.Newf("read operation cancelled").Wrap(ctx.Err())
		}

		// If we got data successfully, break out of retry loop
		if lastErr == nil {
			break
		}

		// Last attempt, don't wait
		if attempt == config.RetryMaxAttempts-1 {
			break
		}

		// Wait before retry with exponential backoff (per architecture pattern)
		select {
		case <-ctx.Done():
			return nil, errfmt.Newf("read operation cancelled").Wrap(ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * config.RetryBackoffFactor)
		if delay > config.RetryMaxDelay {
			delay = config.RetryMaxDelay
		}
	}

	// Check if we failed after all retries
	if lastErr != nil {
		return nil, errfmt.Errorf("read operation failed after %d attempts: %w", config.RetryMaxAttempts, lastErr)
	}

	return nil, errfmt.Newf("read operation failed").Wrap(lastErr)
}

// statFileWithTimeout gets file info asynchronously with a timeout and retry logic
// Follows async I/O best practices: non-blocking goroutine + timeout + retry with exponential backoff
// Per concurrency-patterns-v1.0.md and established RetryConfig pattern from pkg/storage/operation_helper.go
func (ll *LifecycleLoader) statFileWithTimeout(ctx context.Context, filePath string, timeout time.Duration) (os.FileInfo, error) {
	// Use established RetryConfig pattern (matches pkg/storage/operation_helper.go)
	config := DefaultLifecycleIOConfig()
	config.IOTimeout = timeout // Use provided timeout

	var lastErr error
	delay := config.RetryInitialDelay

	// Execute Stat with retry logic (per architecture requirements)
	for attempt := 0; attempt < config.RetryMaxAttempts; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return nil, errfmt.Newf("stat operation cancelled").Wrap(ctx.Err())
		}

		// Non-blocking Stat with timeout (per IO + Async On-Demand pattern)
		type statResult struct {
			info os.FileInfo
			err  error
		}
		resultChan := make(chan statResult, 1)

		// Execute Stat in goroutine to prevent blocking
		goroutinelabels.NewGoroutine("lifecycle_loader_stat_file", fmt.Sprintf("statting lifecycle file %s with timeout (attempt %d/%d)", filepath.Base(filePath), attempt+1, config.RetryMaxAttempts)).
			WithContext(ctx).
			StartSimple(func() {
				info, err := os.Stat(filePath)
				resultChan <- statResult{info: info, err: err}
			})

		// Wait for result with timeout (non-blocking IO pattern)
		select {
		case result := <-resultChan:
			if result.err == nil {
				// Success - return immediately
				return result.info, nil
			}
			lastErr = result.err
			// Check if error is retryable (timeout errors are retryable, but file not found is not)
			if !isRetryableFileError(result.err) {
				return nil, result.err
			}
		case <-time.After(config.IOTimeout):
			// Stat timeout - retryable error
			lastErr = errfmt.Errorf("stat operation timed out after %v", config.IOTimeout)
		case <-ctx.Done():
			// Context cancelled - non-retryable
			return nil, errfmt.Newf("stat operation cancelled").Wrap(ctx.Err())
		}

		// If we got info successfully, break out of retry loop
		if lastErr == nil {
			break
		}

		// Last attempt, don't wait
		if attempt == config.RetryMaxAttempts-1 {
			break
		}

		// Wait before retry with exponential backoff (per architecture pattern)
		select {
		case <-ctx.Done():
			return nil, errfmt.Newf("stat operation cancelled").Wrap(ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * config.RetryBackoffFactor)
		if delay > config.RetryMaxDelay {
			delay = config.RetryMaxDelay
		}
	}

	// Check if we failed after all retries
	if lastErr != nil {
		return nil, errfmt.Errorf("stat operation failed after %d attempts: %w", config.RetryMaxAttempts, lastErr)
	}

	return nil, errfmt.Newf("stat operation failed").Wrap(lastErr)
}

// isRetryableFileError determines if a file I/O error is retryable
// Per architecture: timeout errors and temporary errors are retryable
// Permission errors and file not found are NOT retryable
func isRetryableFileError(err error) bool {
	if err == nil {
		return false
	}

	// File not found is NOT retryable (file doesn't exist, won't exist on retry)
	if os.IsNotExist(err) {
		return false
	}

	// Permission errors are NOT retryable (won't change on retry)
	if os.IsPermission(err) {
		return false
	}

	errStr := err.Error()
	// Timeout errors are retryable
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
		return true
	}
	// Temporary errors are retryable
	if strings.Contains(errStr, "temporary") {
		return true
	}
	// By default, don't retry (safer)
	return false
}
