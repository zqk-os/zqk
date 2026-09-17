package objects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// ErrLifecycleStatusUnknown means the kind's lifecycle loaded but status (after
// built-in aliases and status_mapping) is not a Status entry.
var ErrLifecycleStatusUnknown = errors.New("lifecycle status unknown")

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
	Value       string `yaml:"value"`
	Display     string `yaml:"display"`
	Origin      bool   `yaml:"origin,omitempty"`
	Terminal    bool   `yaml:"terminal,omitempty"`
	Preliminary bool   `yaml:"preliminary,omitempty"`
	Archive     bool   `yaml:"archive,omitempty"`
	WorkDone    bool   `yaml:"work_done,omitempty"`
	Satisfied   bool   `yaml:"satisfied,omitempty"`
	System      bool   `yaml:"system,omitempty"`
	// Role is the cross-kind semantic class (shovel_ready, execution_locked, realign, halted, …).
	// TRACK: [REDACTED-ID]
	Role          string          `yaml:"role,omitempty"`
	Preconditions []string        `yaml:"preconditions,omitempty"`
	Description   string          `yaml:"description,omitempty"`
	Shockwave     ShockwavePolicy `yaml:"shockwave,omitempty"`
}

// StringOrSlice unmarshals a YAML string or list of strings into []string.
type StringOrSlice []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		*s = nil
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		*s = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var arr []string
		if err := value.Decode(&arr); err != nil {
			return err
		}
		*s = arr
		return nil
	default:
		return errfmt.Errorf("StringOrSlice: expected string or sequence, got kind %d", value.Kind)
	}
}

// DependentStatusTrigger matches a one-level dependency_ref shockwave
// (dependent kind reaches one of To). TRACK: [REDACTED-ID]
type DependentStatusTrigger struct {
	Kind string        `yaml:"kind"`
	To   StringOrSlice `yaml:"to"`
}

// TransitionSideEffect is a field mutation applied when a transition fires.
type TransitionSideEffect struct {
	Clear string `yaml:"clear,omitempty"`
}

// Transition represents a valid state transition
type Transition struct {
	From             string   `yaml:"from"`
	To               string   `yaml:"to"`
	Description      string   `yaml:"description"`
	Manual           bool     `yaml:"manual"`
	Auto             bool     `yaml:"auto"`
	Catalyst         string   `yaml:"catalyst,omitempty"`
	ClassVsSpecialty string   `yaml:"class_vs_specialty,omitempty"`
	Preconditions    []string `yaml:"preconditions,omitempty"`
	// Postconditions are declarative hold-after-hop claims (role, cleared fields).
	// Evaluators are not required yet; contract tests and the state-machine rubric
	// treat them as the exam the hop must leave true. TRACK: BLI-1785439369431933000-f0cccd6c
	Postconditions    []string                `yaml:"postconditions,omitempty"`
	OnDependentStatus *DependentStatusTrigger `yaml:"on_dependent_status,omitempty"`
	SideEffects       []TransitionSideEffect  `yaml:"side_effects,omitempty"`
	Shockwave         ShockwavePolicy         `yaml:"shockwave,omitempty"`
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
		lifecyclesDir = normalizeLifecyclesDirIfProjectRoot(lifecyclesDir)
		ll.lifecyclesDir.Store(lifecyclesDir)
	}
	return ll
}

// getLifecyclesDir returns the lazily-loaded lifecycles directory.
func (ll *LifecycleLoader) getLifecyclesDir() string {
	v := ll.lifecyclesDir.Load()
	if v != nil && v.(string) != emptyValue {
		return v.(string)
	}
	if testRoot := zqkenv.TestRoot().Get(); testRoot != emptyValue {
		return findLifecyclesDir()
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

// doEnsureReady runs once per loader; sets lifecyclesDir if empty, preloads
// base_object, then typed-parses every lifecycle YAML (and sibling startup
// configs) so smashed files Warn at CLI init instead of only in contract tests.
// TRACK: BLI-CEF-R26-REMAINING-KINDS-001
func (ll *LifecycleLoader) doEnsureReady(ctx context.Context) error { //nolint:unparam // ctx for LoadFn signature
	dir := ll.getLifecyclesDir()
	_, _ = ll.LoadLifecycle(KindBaseObject)

	lcIssues, err := ParseLifecycleYAMLDir(dir)
	if err != nil {
		return err
	}
	warnLifecycleYAMLIssues("lifecycle", lcIssues)

	cfgDir := filepath.Join(filepath.Dir(dir), filepath.Base(paths.ProcessInternalConfigsDir))
	var cfgIssues []LifecycleYAMLIssue
	if st, statErr := fileutil.Stat(cfgDir); statErr == nil && st.IsDir() {
		var cfgErr error
		cfgIssues, cfgErr = ParseConfigYAMLDir(cfgDir)
		if cfgErr != nil {
			return cfgErr
		}
		warnLifecycleYAMLIssues("config", cfgIssues)
	}

	all := append([]LifecycleYAMLIssue{}, lcIssues...)
	all = append(all, cfgIssues...)
	return FormatLifecycleYAMLIssues(all)
}

// findLifecyclePath resolves the path to {kind}_lifecycle.yaml directly or in subdirectories
func (ll *LifecycleLoader) findLifecyclePath(currentDir, kind string) string {
	lifecycleFile := fmt.Sprintf("%s_lifecycle.yaml", kind)
	direct := filepath.Join(currentDir, lifecycleFile)
	if _, err := fileutil.Stat(direct); err == nil {
		return direct
	}
	for _, domain := range []string{"dna", "kernel", "pm", "qa", "agent", "platform"} {
		candidate := filepath.Join(currentDir, domain, lifecycleFile)
		if _, err := fileutil.Stat(candidate); err == nil {
			return candidate
		}
	}
	var found string
	_ = filepath.WalkDir(currentDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || found != "" {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "bin" || name == ".zqk" || name == "docs" || name == "pkg" || name == "cmd" {
				return filepath.SkipDir
			}
		}
		if !d.IsDir() && d.Name() == lifecycleFile {
			found = path
		}
		return nil
	})
	if found != "" {
		return found
	}
	return direct
}

// LoadLifecycle loads a lifecycle definition for a given object kind
// If no specific lifecycle exists for the kind, falls back to base_object_lifecycle.yaml
func (ll *LifecycleLoader) LoadLifecycle(kind string) (*Lifecycle, error) {
	currentDir := ll.getLifecyclesDir()
	lifecyclePath := ll.findLifecyclePath(currentDir, kind)

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
		if fileutil.IsNotExist(err) {
			baseLifecyclePath := ll.findLifecyclePath(currentDir, KindBaseObject)
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

	newCached := &cachedLifecycle{lifecycle: &lifecycle, mtime: mtime}
	if actual, loaded := ll.cache.LoadOrStore(kind, newCached); loaded {
		cachedAfter := actual.(*cachedLifecycle)
		statAfter, statErr := ll.statFileWithTimeout(pkgctx.NewSystemContext(), lifecyclePath, 2*time.Second)
		if statErr == nil && statAfter != nil && !statAfter.ModTime().After(cachedAfter.mtime) {
			return cachedAfter.lifecycle, nil
		}
		ll.cache.Store(kind, newCached)
	}
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

// StatusesForRole returns this kind's status values whose lifecycle role is any of roles, in
// lifecycle declaration order. Returns nil, nil when the kind has no lifecycle.
//
// This is the reverse of [StatusChecker.Role], and its absence is why so many callers hand-maintain
// literal status slices to answer role questions ("which plan statuses are shovel-ready?"). Those
// slices cannot be checked against anything and have drifted: contractchange listed a priority_plan
// status "ready" that no lifecycle has ever defined, and several terminal lists name backlog_item
// "rejected", which is an alias for archived and so never matches a persisted status. Derive the
// labels from the role instead of restating them.
func (ll *LifecycleLoader) StatusesForRole(kind string, roles ...string) ([]string, error) {
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return nil, err
	}
	if len(lifecycle.Statuses) == 0 || len(roles) == 0 {
		return nil, nil
	}
	want := make(map[string]bool, len(roles))
	for _, r := range roles {
		want[strings.TrimSpace(r)] = true
	}
	var out []string
	for _, s := range lifecycle.Statuses {
		if want[strings.TrimSpace(s.Role)] {
			out = append(out, s.Value)
		}
	}
	return out, nil
}

// IsRepairParkStatus reports whether status is a park landing used to re-enter
// the state machine from an illegally persisted status.
func IsRepairParkStatus(status string) bool {
	switch status {
	case ObjectStatusDeferred, ObjectStatusRoadmap, ObjectStatusArchived:
		return true
	default:
		return false
	}
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

	toValid, err := ll.IsValidStatus(kind, toCanonical)
	if err != nil {
		return false, err
	}
	if !toValid {
		return false, errfmt.Errorf("invalid target status: %s", to)
	}

	if !fromValid {
		// Illegally persisted from: allow re-entry onto a named park status.
		// TRACK: BLI-KERNEL-UNPAIRED-DELETE-INBOUND-001
		if IsRepairParkStatus(toCanonical) {
			return true, nil
		}
		return false, errfmt.Errorf("invalid source status: %s", from)
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
// Use when persisting (e.g. Create/Update) so stored values are canonical.
// Unknown statuses fail closed (ErrLifecycleStatusUnknown) — do not persist
// values the kind's lifecycle does not name.
func (ll *LifecycleLoader) NormalizeStatusForKind(kind, status string) (string, error) {
	if status == emptyValue {
		return status, nil
	}
	resolved, err := ll.ResolveStatusForKind(kind, status)
	if err != nil {
		return status, err
	}
	return resolved.Value, nil
}

// ResolveStatusForKind returns the lifecycle Status entry for kind after
// alias-normalizing status (built-in aliases + kind status_mapping). It does not
// interpret policy (CASable, draft-plane, git hooks, etc.) — callers decide.
func (ll *LifecycleLoader) ResolveStatusForKind(kind, status string) (Status, error) {
	if status == emptyValue {
		return Status{}, errfmt.Errorf("empty status")
	}
	lifecycle, err := ll.LoadLifecycle(kind)
	if err != nil {
		return Status{}, err
	}
	validSet := make(map[string]bool, len(lifecycle.Statuses))
	for _, s := range lifecycle.Statuses {
		validSet[s.Value] = true
	}
	canonical := ApplyAliasesForStatus(status, validSet)
	canonical = applyLifecycleStatusMapping(lifecycle.StatusMapping, status, canonical, validSet)
	for _, s := range lifecycle.Statuses {
		if s.Value == canonical {
			return s, nil
		}
	}
	return Status{}, errfmt.Newf("status %q not found in lifecycle for kind %q", status, kind).Wrap(ErrLifecycleStatusUnknown)
}

// applyLifecycleStatusMapping remaps parent/legacy status keys from status_mapping
// onto child lifecycle values when the input is not already a valid status for the
// kind. Mapping is inheritance/legacy aliasing — never collapse two sibling statuses
// that both exist (e.g. priority_plan in_progress vs active).
func applyLifecycleStatusMapping(mapping map[string]string, raw, afterBuiltin string, validSet map[string]bool) string {
	if len(mapping) == 0 {
		return afterBuiltin
	}
	if validSet[afterBuiltin] {
		return afterBuiltin
	}
	if validSet[raw] {
		return raw
	}
	for _, key := range []string{afterBuiltin, raw} {
		if mapped, ok := mapping[key]; ok && validSet[mapped] {
			return mapped
		}
	}
	return afterBuiltin
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
	resolved, err := ll.ResolveStatusForKind(kind, status)
	if err != nil {
		if errors.Is(err, ErrLifecycleStatusUnknown) {
			return false, nil
		}
		return false, err
	}
	return resolved.Terminal, nil
}

// IsPreliminaryStatusForKind reports whether status is preliminary/draft for kind.
// Trusts the lifecycle status `preliminary` flag only. Origin alone must not imply
// draft (born-complete metrics use origin+terminal with preliminary:false; treating
// origin as draft parked them forever with no leave path).
// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane / promote membrane.
func (ll *LifecycleLoader) IsPreliminaryStatusForKind(kind, status string) (bool, error) {
	if status == emptyValue {
		return false, nil
	}
	resolved, err := ll.ResolveStatusForKind(kind, status)
	if err != nil {
		if errors.Is(err, ErrLifecycleStatusUnknown) {
			return false, nil
		}
		return false, err
	}
	if resolved.Preliminary {
		return true, nil
	}
	switch resolved.Value {
	case "conceptual", "exploring", "identified", "draft", "proposed":
		return true, nil
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
	if testRoot := zqkenv.TestRoot().Get(); testRoot != emptyValue {
		lcDir := filepath.Join(testRoot, paths.ProcessInternalLifecyclesDir)
		if info, err := fileutil.Stat(lcDir); err == nil && info.IsDir() && lifecyclesDirHasBaseLifecycle(lcDir) {
			return lcDir
		}
	}
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
				info fileutil.FileInfo
				err  error
			}
			resultChan := make(chan statResult, 1)

			goroutinelabels.NewGoroutine("lifecycle_loader_find_dir", fmt.Sprintf("checking directory %s (attempt %d/%d)", dir, attempt+1, config.RetryMaxAttempts)).
				WithContext(ctx).
				StartSimple(func() {
					info, err := fileutil.Stat(dir)
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
				if fileutil.IsNotExist(result.err) {
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

// normalizeLifecyclesDirIfProjectRoot normalizes a project root to its internal lifecycles dir.
func normalizeLifecyclesDirIfProjectRoot(lifecyclesDir string) string {
	if lifecyclesDir == emptyValue {
		return ""
	}
	clean := filepath.Clean(lifecyclesDir)
	if lifecyclesDirHasBaseLifecycle(clean) {
		return clean
	}
	nested := filepath.Join(clean, paths.ProcessInternalLifecyclesDir)
	if lifecyclesDirHasBaseLifecycle(nested) {
		return nested
	}
	return clean
}

// lifecyclesDirHasBaseLifecycle checks if base_object_lifecycle.yaml exists directly or in known domains.
func lifecyclesDirHasBaseLifecycle(dir string) bool {
	baseFile := fmt.Sprintf("%s_lifecycle.yaml", KindBaseObject)
	if _, err := fileutil.Stat(filepath.Join(dir, baseFile)); err == nil {
		return true
	}
	for _, domain := range []string{"dna", "kernel", "pm", "qa", "agent", "platform"} {
		if _, err := fileutil.Stat(filepath.Join(dir, domain, baseFile)); err == nil {
			return true
		}
	}
	return false
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
	var kind string
	if lifecycleName == "base_lifecycle" || lifecycleName == "base_object_lifecycle" {
		kind = KindBaseObject
	} else if strings.HasSuffix(lifecycleName, "_lifecycle") {
		kind = strings.TrimSuffix(lifecycleName, "_lifecycle")
	} else {
		kind = lifecycleName
	}

	lifecyclePath := ll.findLifecyclePath(ll.getLifecyclesDir(), kind)
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

	// Child-declared status values (siblings). status_mapping must not collapse
	// two names that both exist on the child (e.g. agent_task approved≠in_progress).
	childStatusSet := make(map[string]bool, len(child.Statuses))
	for _, status := range child.Statuses {
		if status.Value != emptyValue {
			childStatusSet[status.Value] = true
		}
	}
	mapInheritedStatus := func(internal string) string {
		if internal == emptyValue || internal == "*" {
			return internal
		}
		external := merged.StatusMapping[internal]
		if external == emptyValue {
			return internal
		}
		// Keep identity when both ends are sibling statuses on the child.
		if childStatusSet[internal] && childStatusSet[external] && internal != external {
			return internal
		}
		return external
	}

	// Start with parent statuses, then add/override with child statuses
	statusMap := make(map[string]Status)
	for _, status := range parent.Statuses {
		externalValue := mapInheritedStatus(status.Value)
		status.Value = externalValue
		statusMap[externalValue] = status
	}

	// Override/add child statuses (shockwave inherits when the child omits it)
	for _, status := range child.Statuses {
		if prev, ok := statusMap[status.Value]; ok {
			status.Shockwave = MergeShockwavePolicy(prev.Shockwave, status.Shockwave)
		}
		statusMap[status.Value] = status
	}

	// Convert map to slice
	for _, status := range statusMap {
		merged.Statuses = append(merged.Statuses, status)
	}

	// Start with parent transitions, then add/override with child transitions
	transitionMap := make(map[string]Transition) // key: "from->to"
	for _, transition := range parent.Transitions {
		from := transition.From
		if from != "*" {
			from = mapInheritedStatus(from)
		}
		to := transition.To
		if to != "*" {
			to = mapInheritedStatus(to)
		}
		transition.From = from
		transition.To = to
		key := fmt.Sprintf("%s->%s", from, to)
		transitionMap[key] = transition
	}

	// Override/add child transitions (shockwave inherits when the child omits it)
	for _, transition := range child.Transitions {
		key := fmt.Sprintf("%s->%s", transition.From, transition.To)
		if prev, ok := transitionMap[key]; ok {
			transition.Shockwave = MergeShockwavePolicy(prev.Shockwave, transition.Shockwave)
		}
		transitionMap[key] = transition
	}

	// Convert map to slice deterministically
	keys := make([]string, 0, len(transitionMap))
	for k := range transitionMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		merged.Transitions = append(merged.Transitions, transitionMap[k])
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
				data, err := fileutil.ReadFile(filePath)
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

// statFileWithTimeout stats a local lifecycle YAML. LoadLifecycle cache-hit used to
// spawn a labeled goroutine + time.After per Stat; system-check CPU attributed that
// to pthread_cond / usleep while validating thousands of objects.
func (ll *LifecycleLoader) statFileWithTimeout(ctx context.Context, filePath string, timeout time.Duration) (fileutil.FileInfo, error) {
	_ = timeout
	if err := ctx.Err(); err != nil {
		return nil, errfmt.Newf("stat operation cancelled").Wrap(err)
	}
	info, err := fileutil.Stat(filePath)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// isRetryableFileError determines if a file I/O error is retryable
// Per architecture: timeout errors and temporary errors are retryable
// Permission errors and file not found are NOT retryable
func isRetryableFileError(err error) bool {
	if err == nil {
		return false
	}

	// File not found is NOT retryable (file doesn't exist, won't exist on retry)
	if fileutil.IsNotExist(err) {
		return false
	}

	// Permission errors are NOT retryable (won't change on retry)
	if fileutil.IsPermission(err) {
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
