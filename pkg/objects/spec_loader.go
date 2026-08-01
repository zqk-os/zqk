package objects

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	yamlspec "github.com/lanceman/zqk/pkg/specbuilder/yaml"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

// Spec represents a loaded object specification with resolved inheritance.
// Location is the path reference (prefix: or abs:) for the backing YAML file so it can be re-used and resolved where necessary.
type Spec struct {
	SchemaVersion string `yaml:"schema_version"`
	Ontology      string `yaml:"ontology"`
	Extends       string `yaml:"extends"`
	Visibility    string `yaml:"visibility"`
	Description   string `yaml:"description"`
	// IDPrefixes defines the valid ID prefixes for this kind (e.g., ["GOAL-"]).
	// Derives id_prefixes_config.yaml entries during spec generation.
	IDPrefixes []string `yaml:"id_prefixes,omitempty"`
	// IDSynonyms defines standardized aliases for this kind (e.g., ["backlog"]).
	// Derives id_prefixes_config.yaml synonyms during spec generation.
	IDSynonyms []string `yaml:"id_synonyms,omitempty"`
	// StorageProfile is the data-cell storage mechanism for this kind (optional; inherited from parent when empty).
	// See pkg/datacell.StorageProfile and DATA_CELL_MODEL.md.
	StorageProfile string         `yaml:"storage_profile,omitempty"`
	Traits         []string       `yaml:"traits"`
	ExcludeTraits  []string       `yaml:"exclude_traits,omitempty"`
	Fields         map[string]any `yaml:"fields"`
	// Resolved fields after inheritance
	ResolvedFields map[string]any `yaml:"-"`
	ResolvedTraits []string       `yaml:"-"`
	// Location is set by the loader to a path reference (prefix:relPath or abs:absolutePath) for the backing YAML file.
	Location string `yaml:"-"`
}

// SpecResolvedFieldsMissing reports whether spec is nil or has no resolved fields map (e.g. not loaded).
func SpecResolvedFieldsMissing(spec *Spec) bool {
	return spec == nil || spec.ResolvedFields == nil
}

// cachedSpec stores a spec with its file modification time for staleness detection
type cachedSpec struct {
	spec  *Spec
	mtime time.Time // File modification time when cached
}

// VersionedBuilderRegistryInterface allows SpecLoader to use versioned builders without import cycles
// This interface should be implemented by pkg/specbuilder/builders.VersionedBuilderRegistry
type VersionedBuilderRegistryInterface interface {
	GetBuilder(ontology, version string) (SpecBuilderInterface, error)
	GetLatestVersion(ontology string) (string, error)
	GetVersions(ontology string) []string
}

// SpecBuilderInterface allows SpecLoader to use spec builders without import cycles
// This interface should be implemented by pkg/specbuilder/builders.SpecBuilder
type SpecBuilderInterface interface {
	Build() *Spec
	GetVersion() string
	GetOntology() string
}

// builderRegistryHolder wraps the versioned registry for atomic.Pointer (atomic stores need a concrete type).
type builderRegistryHolder struct {
	reg VersionedBuilderRegistryInterface
}

// fileContentEntry holds raw file bytes and mtime for staleness detection
type fileContentEntry struct {
	data  []byte
	mtime time.Time
}

// specShard represents a shard of the spec cache
// Uses sync.Map for lock-free reads (cache hits are hot path)
// sync.Map handles all synchronization internally - no mutex needed
type specShard struct {
	cache            sync.Map // map[string]*cachedSpec - ontology -> cached spec
	ontologyCache    sync.Map // map[string]string - file path -> ontology (for getOntologyFromFile)
	pathCache        sync.Map // map[string]*cachedSpec - absolute path -> cached spec (avoids double ReadFile on first load)
	fileContentCache sync.Map // map[string]*fileContentEntry - absolute path -> content+mtime (avoids double ReadFile; re-read if mtime changed)
}

// SpecLoader loads and resolves object specifications with inheritance.
// Uses sharded locking to reduce contention when multiple goroutines load different specs concurrently.
// Optional EnsureReady(ctx) uses the component loader pattern (pkg/loader) to warm base specs once with timeout.
type SpecLoader struct {
	specsDir        string
	dependencyGraph *SpecDependencyGraph                  // Optional: for load order validation
	shards          []*specShard                          // Sharded caches (reduces lock contention)
	shardCount      int                                   // Number of shards (power of 2 for fast modulo)
	builderRegistry atomic.Pointer[builderRegistryHolder] // Optional: for version-aware loading (atomic load/store; no mutex)
	globalMu        sync.RWMutex                          // Protects specsDir updates in ensure-ready (see doEnsureReady)
	metrics         *SpecLoaderMetrics                    // Metrics for lock operations
	logger          concurrency.LockLogger                // Logger for lock operations
	readyRunner     *loader.Runner                        // Optional: one-shot "ensure ready" (warm base specs)
	readyRunnerOnce sync.Once
	cacheBuffer     *SpecCacheUpdateBuffer // Unused (removed batching - sync.Map.Store() is already optimized)
	// cacheRevision increments on ClearCache / InvalidateSpec / InvalidateSpecByFile so callers
	// can correlate derived materializations (spec index, kind lists) with invalidation (see SPEC_ORIGIN_PLANE.md).
	cacheRevision atomic.Uint64
	// specStorage backs spec reads (CRIT-9031 / SpecStorageProvider). When nil, readSpecFile uses os.ReadFile.
	specStorage SpecStorageProvider
}

// NewSpecLoader creates a new spec loader
// Uses sharded locking (64 shards) to reduce contention when loading specs concurrently
//
// If specsDir is empty, the object-specs directory is discovered via findSpecsDir().
// If specsDir is a project root (caller passed the repo/workspace root rather than
// docs/architecture/_internal/object_specs), it is normalized when that nested tree
// contains base_object.yaml so relative spec names like backlog_item.yaml resolve
// correctly. Spec reads go through FileSpecStorageProvider (non-nil) when the
// resolved specs directory is non-empty.
func NewSpecLoader(specsDir string) *SpecLoader {
	if specsDir == emptyValue {
		specsDir = findSpecsDir()
	} else {
		specsDir = normalizeSpecsDirIfProjectRoot(specsDir)
	}

	// Use 64 shards (power of 2) for better distribution and reduced lock contention
	// Increased from 16 to 64 to handle high concurrency (14k+ objects) better
	// Each shard uses sync.Map for lock-free cache reads (cache hits are hot path)
	shardCount := 64
	shards := make([]*specShard, shardCount)
	for i := range shardCount {
		shards[i] = &specShard{
			// sync.Map initialized as zero value (ready to use)
		}
	}

	loader := &SpecLoader{
		specsDir:   specsDir,
		shards:     shards,
		shardCount: shardCount,
		metrics:    GetGlobalSpecLoaderMetrics(),
		logger:     &specLoaderLockLogger{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))},
	}
	// Initialize dependency graph for validation
	loader.dependencyGraph = NewSpecDependencyGraph(loader)
	// Removed write-behind buffer: sync.Map.Store() is already optimized for concurrent access.
	// Batching doesn't reduce Store() calls (no deduplication), and adds channel/worker overhead.
	// If Store() is truly blocking, batching won't help - we still call it the same number of times.
	loader.cacheBuffer = nil
	if specsDir != emptyValue {
		if p, err := NewFileSpecStorageProvider(specsDir); err == nil {
			loader.specStorage = p
		}
	}
	return loader
}

// readSpecFile loads raw bytes and modification time for an absolute spec path.
func (sl *SpecLoader) readSpecFile(ctx context.Context, absPath string) ([]byte, time.Time, error) {
	if sl != nil && sl.specStorage != nil {
		data, meta, err := sl.specStorage.ReadSpecBytes(ctx, absPath)
		return data, meta.ModTime, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	var mt time.Time
	if st, err2 := os.Stat(absPath); err2 == nil {
		mt = st.ModTime()
	}
	return data, mt, nil
}

// specLoaderLockLogger adapts logging.Logger to concurrency.LockLogger
type specLoaderLockLogger struct {
	logger logging.Logger
}

func (l *specLoaderLockLogger) Debug(msg string, fields ...concurrency.LockField) {
	// Convert LockField to logging fields
	logFields := make([]logging.Field, 0, len(fields))
	for _, f := range fields {
		// Convert value to appropriate logging field type
		switch v := f.Value.(type) {
		case string:
			logFields = append(logFields, logging.String(f.Key, v))
		case int:
			logFields = append(logFields, logging.Int(f.Key, v))
		case int64:
			logFields = append(logFields, logging.Int(f.Key, int(v)))
		case error:
			logFields = append(logFields, logging.Error(v))
		default:
			// For other types, convert to string
			logFields = append(logFields, logging.String(f.Key, fmt.Sprintf("%v", v)))
		}
	}
	entry := logging.Fluent(l.logger).Debug(msg)
	if len(logFields) > 0 {
		entry = entry.WithFields(logFields...)
	}
	entry.Log()
}

func (l *specLoaderLockLogger) Warn(msg string, fields ...concurrency.LockField) {
	// Convert LockField to logging fields
	logFields := make([]logging.Field, 0, len(fields))
	for _, f := range fields {
		// Convert value to appropriate logging field type
		switch v := f.Value.(type) {
		case string:
			logFields = append(logFields, logging.String(f.Key, v))
		case int:
			logFields = append(logFields, logging.Int(f.Key, v))
		case int64:
			logFields = append(logFields, logging.Int(f.Key, int(v)))
		case error:
			logFields = append(logFields, logging.Error(v))
		default:
			// For other types, convert to string
			logFields = append(logFields, logging.String(f.Key, fmt.Sprintf("%v", v)))
		}
	}
	entry := logging.Fluent(l.logger).Warn(msg)
	if len(logFields) > 0 {
		entry = entry.WithFields(logFields...)
	}
	entry.Log()
}

// getShard returns the shard for a given ontology
// Uses simple hash of ontology string for distribution
func (sl *SpecLoader) getShard(ontology string) *specShard {
	// Simple hash function for distribution
	hash := 0
	for _, c := range ontology {
		hash = hash*31 + int(c)
	}
	// Use bitwise AND for fast modulo (shardCount must be power of 2)
	shardIndex := hash & (sl.shardCount - 1)
	return sl.shards[shardIndex]
}

// SetBuilderRegistry sets the builder registry for version-aware loading
// This allows version-aware loading without creating import cycles
func (sl *SpecLoader) SetBuilderRegistry(registry VersionedBuilderRegistryInterface) {
	if registry == nil {
		sl.builderRegistry.Store(nil)
		return
	}
	sl.builderRegistry.Store(&builderRegistryHolder{reg: registry})
}

// getBuilderRegistry returns the registry set by SetBuilderRegistry (may be nil).
// Uses atomic load so hot paths avoid globalMu and WithRLockTimeout (which runs the callback in
// another goroutine and can false-timeout under scheduler delay while "holding rlock").
func (sl *SpecLoader) getBuilderRegistry() VersionedBuilderRegistryInterface {
	h := sl.builderRegistry.Load()
	if h == nil {
		return nil
	}
	return h.reg
}

// getReadyRunner returns the shared loader.Runner for "ensure ready" (lazily created).
func (sl *SpecLoader) getReadyRunner() *loader.Runner {
	sl.readyRunnerOnce.Do(func() {
		sl.readyRunner = loader.NewRunner("spec_loader", func(ctx context.Context) error {
			return sl.doEnsureReady(ctx)
		})
	})
	return sl.readyRunner
}

// EnsureReady ensures the spec loader is ready: specs dir is set and base specs (base_object, auditable) are warmed.
// Uses the component loader pattern (pkg/loader) so concurrent callers wait with timeout instead of each doing I/O.
// Optional: call before heavy use to avoid "ID patterns currently loading" style races; per-spec loading remains lock-free.
func (sl *SpecLoader) EnsureReady(ctx context.Context) error {
	return sl.getReadyRunner().Load(ctx)
}

// doEnsureReady runs once per loader; sets specsDir if empty and preloads base specs.
func (sl *SpecLoader) doEnsureReady(ctx context.Context) error { //nolint:unparam // ctx for LoadFn signature
	_ = concurrency.RunInLockWithLogger(
		&sl.globalMu,
		LockNameSpecLoaderEnsureReadyDir,
		sl.logger,
		func() error {
			if sl.specsDir == emptyValue {
				sl.specsDir = findSpecsDir()
			}
			return nil
		},
	)
	// Warm base specs so dependency graph and common inheritance are cached
	_, _ = sl.LoadSpecWithInheritance("base_object.yaml")
	_, _ = sl.LoadSpecWithInheritance("auditable.yaml")
	return nil
}

// ValidateDependencyGraph validates the spec dependency graph
// Returns errors if cycles or missing parents are detected
func (sl *SpecLoader) ValidateDependencyGraph() []*GraphValidationError {
	if err := sl.dependencyGraph.BuildGraph(); err != nil {
		return []*GraphValidationError{{
			Type:    "build_error",
			Message: err.Error(),
			Specs:   []string{},
		}}
	}
	return sl.dependencyGraph.ValidateGraph()
}

// GetLoadOrder returns specs in topological order (parents before children)
// Validates the graph first and returns error if cycles are detected
func (sl *SpecLoader) GetLoadOrder() ([]*Spec, error) {
	if err := sl.dependencyGraph.BuildGraph(); err != nil {
		return nil, errfmt.Newf("failed to build dependency graph").Wrap(err)
	}

	// Validate before sorting
	errors := sl.dependencyGraph.ValidateGraph()
	if len(errors) > 0 {
		return nil, errfmt.Errorf("dependency graph validation failed: %v", errors)
	}

	return sl.dependencyGraph.TopologicalSort()
}

// LoadSpecWithInheritance loads a spec and resolves its inheritance chain
// Results are cached by path and ontology to avoid reloading the same spec (and double ReadFile on first load)
func (sl *SpecLoader) LoadSpecWithInheritance(specFile string) (*Spec, error) {
	// Resolve spec file path (normalize for path cache)
	specPath := specFile
	if !filepath.IsAbs(specFile) {
		specPath = filepath.Join(sl.specsDir, specFile)
	}
	if abs, err := filepath.Abs(specPath); err == nil {
		specPath = abs // normalize for path cache key
	}

	// OPTIMIZED: Check path cache first to avoid double ReadFile (getOntologyFromFile + loadSpecWithInheritanceRecursive)
	pathShard := sl.getShard(specPath)
	if pathVal, ok := pathShard.pathCache.Load(specPath); ok {
		cached := pathVal.(*cachedSpec)
		if stat, err := os.Stat(specPath); err == nil {
			if stat.ModTime().Equal(cached.mtime) || stat.ModTime().Before(cached.mtime) {
				return cached.spec, nil
			}
		}
	}

	// Get ontology for cache key (read file once; may hit ontologyCache)
	ontology := sl.getOntologyFromFile(specPath)
	if ontology == emptyValue {
		// Fallback: use filename without extension as cache key
		base := filepath.Base(specFile)
		ontology = base[:len(base)-len(filepath.Ext(base))]
	}

	// Get shard for this ontology (reduces contention)
	shard := sl.getShard(ontology)

	// OPTIMIZED: Lock-free cache read using sync.Map (hot path - cache hits)
	// Once cached, reads are completely lock-free - no contention
	cachedVal, exists := shard.cache.Load(ontology)
	if exists {
		cached := cachedVal.(*cachedSpec)
		// Check if file has been modified (I/O outside any lock)
		if stat, err := os.Stat(specPath); err == nil {
			if stat.ModTime().Equal(cached.mtime) || stat.ModTime().Before(cached.mtime) {
				// File hasn't changed, return cached spec (LOCK-FREE PATH)
				return cached.spec, nil
			}
			// File was modified - need to reload (will update cache below)
		}
	}

	// Load spec (will recursively load parents)
	// Pass ontology to avoid re-reading file for circular detection
	visited := make(map[string]bool) // Track visited specs for circular detection
	spec, err := sl.loadSpecWithInheritanceRecursive(specFile, visited, ontology, 0)
	if err != nil {
		return nil, err
	}

	// Get file modification time for staleness detection
	var mtime time.Time
	stat, statErr := os.Stat(specPath)
	when.When(func() bool { return statErr == nil }).Then(func() {
		mtime = stat.ModTime()
	}).OrElse(func() {
		mtime = time.Now()
	}).Run()

	cached := &cachedSpec{spec: spec, mtime: mtime}

	// OPTIMIZED: Lock-free double-checked pattern using sync.Map
	// Another goroutine might have loaded it while we were loading
	cachedAfterLoadVal, existsAfterLoad := shard.cache.Load(ontology)
	if existsAfterLoad {
		cachedAfterLoad := cachedAfterLoadVal.(*cachedSpec)
		// Check if our loaded version is newer than cached
		stat, statErr := os.Stat(specPath)
		when.When(func() bool { return statErr == nil && stat.ModTime().After(cachedAfterLoad.mtime) }).Then(func() {
			// Queue cache updates instead of blocking on Store()
			sl.queueCacheUpdate(shard, CacheUpdateSpec, ontology, cached)
			sl.queueCacheUpdate(pathShard, CacheUpdatePath, specPath, cached)
		}).OrElse(func() {
			spec = cachedAfterLoad.spec
		}).Run()
		return spec, nil
	}

	// Queue cache updates instead of blocking on Store() (write-behind pattern reduces sync.Map contention)
	sl.queueCacheUpdate(shard, CacheUpdateSpec, ontology, cached)
	sl.queueCacheUpdate(pathShard, CacheUpdatePath, specPath, cached)
	return spec, nil
}

// queueCacheUpdate applies cache updates directly using sync.Map.Store()
// sync.Map is optimized for concurrent access - no batching needed
func (sl *SpecLoader) queueCacheUpdate(shard *specShard, updateType CacheUpdateType, key string, value any) {
	switch updateType {
	case CacheUpdateSpec:
		shard.cache.Store(key, value)
	case CacheUpdatePath:
		shard.pathCache.Store(key, value)
	case CacheUpdateOntology:
		shard.ontologyCache.Store(key, value)
	case CacheUpdateFileContent:
		shard.fileContentCache.Store(key, value)
	}
}

// getShardByPath returns the shard for a given file path (for path-based caches like fileContentCache).
func (sl *SpecLoader) getShardByPath(filePath string) *specShard {
	hash := 0
	for _, c := range filePath {
		hash = hash*31 + int(c)
	}
	shardIndex := hash & (sl.shardCount - 1)
	return sl.shards[shardIndex]
}

// maxSpecInheritanceDepth limits recursion to prevent hang from cycles or bugs (see GENERATE_BUILDERS_SAMPLE_ANALYSIS.md)
const maxSpecInheritanceDepth = 30

// pathRefForSpecFile returns a path reference (prefix:relPath or abs:absolutePath) for the given spec file path.
// Enables spec-like objects backed by YAML to reference their location for reuse and resolution.
func pathRefForSpecFile(filePath string) string {
	if filePath == emptyValue {
		return ""
	}
	if filepath.IsAbs(filePath) {
		return paths.PathSchemeAbs + filePath
	}
	return paths.PathRefFromRelPath(filePath)
}

// loadSpecWithInheritanceRecursive recursively loads spec and resolves inheritance
// This is called by LoadSpecWithInheritance which handles caching
// ontology parameter is the ontology of the current spec (already known from parent call)
// depth is the current recursion depth (0 = top level); used to cap recursion and avoid unbounded stack growth.
func (sl *SpecLoader) loadSpecWithInheritanceRecursive(specFile string, visited map[string]bool, ontology string, depth int) (*Spec, error) {
	if depth > maxSpecInheritanceDepth {
		return nil, errfmt.Errorf("spec inheritance depth exceeded %d (possible cycle or bug): %s", maxSpecInheritanceDepth, specFile)
	}
	// Resolve spec file path to absolute for cache key consistency
	specPath := specFile
	if !filepath.IsAbs(specFile) {
		specPath = filepath.Join(sl.specsDir, specFile)
	}
	if abs, err := filepath.Abs(specPath); err == nil {
		specPath = abs
	}

	// If ontology not provided, read it from file (for parent specs)
	if ontology == emptyValue {
		ontology = sl.getOntologyFromFile(specPath)
	}

	// Check if this spec is already fully merged and cached
	if ontology != emptyValue {
		shard := sl.getShard(ontology)
		if cachedVal, exists := shard.cache.Load(ontology); exists {
			cached := cachedVal.(*cachedSpec)
			if stat, err := os.Stat(specPath); err == nil {
				if stat.ModTime().Equal(cached.mtime) || stat.ModTime().Before(cached.mtime) {
					return cached.spec, nil
				}
			}
		}
	}

	// Check for circular inheritance
	if ontology != emptyValue {
		if visited[ontology] {
			return nil, errfmt.Errorf("circular inheritance detected: %s", ontology)
		}
		visited[ontology] = true
	}

	// Load the spec file: use fileContentCache if getOntologyFromFile already read it (avoids second ReadFile)
	pathShard := sl.getShardByPath(specPath)
	var data []byte
	if cached, ok := pathShard.fileContentCache.Load(specPath); ok {
		entry := cached.(*fileContentEntry)
		if stat, err := os.Stat(specPath); err == nil && !stat.ModTime().After(entry.mtime) {
			data = entry.data
		}
	}
	if data == nil {
		rd, mtime, err := sl.readSpecFile(context.Background(), specPath)
		if err != nil {
			return nil, errfmt.Errorf("failed to read spec file %s: %w", specPath, err)
		}
		data = rd
		// Queue cache update instead of blocking on Store()
		sl.queueCacheUpdate(pathShard, CacheUpdateFileContent, specPath, &fileContentEntry{data: data, mtime: mtime})
	}

	// Validate against JSON schema if $schema is present (optional validation)
	// Skip when ZQK_SKIP_SPEC_SCHEMA_VALIDATION=1 to avoid deep recursion/hang in generate-builders (GENERATE_BUILDERS_SAMPLE_ANALYSIS.md)
	var tempSpec map[string]any
	if os.Getenv(zqkenv.SkipSpecSchemaValidation()) != "1" && yaml.Unmarshal(data, &tempSpec) == nil {
		if schemaRef, ok := tempSpec["$schema"].(string); ok && schemaRef != emptyValue {
			// Try to find schemas directory relative to project root
			schemasDir := ".zqk/cli/specs/schemas"
			// Try to resolve from specsDir
			if sl.specsDir != emptyValue {
				dir := sl.specsDir
				for i := 0; i < 10; i++ { // Limit depth
					testPath := filepath.Join(dir, "..", "..", "..", paths.ProjectDataDir, "cli", "specs", "schemas")
					if abs, err := filepath.Abs(testPath); err == nil {
						if _, err := os.Stat(abs); err == nil {
							schemasDir = abs
							break
						}
					}
					parent := filepath.Dir(dir)
					if parent == dir {
						break
					}
					dir = parent
				}
			}
			validator := yamlspec.NewSchemaValidator(schemasDir)
			if err := validator.ValidateYAML(specPath, schemaRef); err != nil {
				// Log warning but don't fail (schema validation is optional for now)
				// In the future, this could be made strict
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(logger).Warn("Schema validation failed for spec").
					String("spec_path", specPath).
					String("schema_ref", schemaRef).
					WithError(err).
					Log()
				// Uncomment to make schema validation strict:
				// return nil, stdErrors.New(errfmt.Newf("schema validation failed for %s: %w", specPath, err).Build())
			}
		}
	}

	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Errorf("failed to parse spec file %s: %w", specPath, err)
	}

	// Normalize "null" extends to empty string (YAML "null" is a valid value but we treat it as no parent)
	if spec.Extends == "null" {
		spec.Extends = ""
	}

	// Initialize resolved fields and traits with current spec's values
	// ResolvedFields stores the complete field definitions (including type, validation, checklist, etc.)
	spec.ResolvedFields = make(map[string]any)
	if spec.Fields != nil {
		// Fields is a map of field definitions, each containing type, validation, checklist, etc.
		for k, v := range spec.Fields {
			spec.ResolvedFields[k] = v
		}
	}
	spec.ResolvedTraits = make([]string, len(spec.Traits))
	copy(spec.ResolvedTraits, spec.Traits)

	// If this spec extends another, load and merge parent
	if spec.Extends != emptyValue && spec.Extends != "null" {
		parentFile := spec.Extends + ".yaml"
		// Pass empty ontology - will be read from file for parent
		parentSpec, err := sl.loadSpecWithInheritanceRecursive(parentFile, visited, "", depth+1)
		if err != nil {
			return nil, errfmt.Errorf("failed to load parent spec %s: %w", parentFile, err)
		}

		// Merge parent fields (child overrides parent)
		for k, v := range parentSpec.ResolvedFields {
			if _, exists := spec.ResolvedFields[k]; !exists {
				spec.ResolvedFields[k] = v
			}
		}

		// Merge traits (union - no duplicates)
		traitMap := make(map[string]bool)
		for _, trait := range spec.ResolvedTraits {
			traitMap[trait] = true
		}
		for _, trait := range parentSpec.ResolvedTraits {
			if !traitMap[trait] {
				spec.ResolvedTraits = append(spec.ResolvedTraits, trait)
				traitMap[trait] = true
			}
		}
		// Merge exclude_traits (union - no duplicates)
		excludedMap := make(map[string]bool)
		for _, t := range spec.ExcludeTraits {
			excludedMap[t] = true
		}
		for _, t := range parentSpec.ExcludeTraits {
			if !excludedMap[t] {
				spec.ExcludeTraits = append(spec.ExcludeTraits, t)
				excludedMap[t] = true
			}
		}

		if spec.StorageProfile == "" && parentSpec.StorageProfile != "" {
			spec.StorageProfile = parentSpec.StorageProfile
		}
	}
	applyTraitExclusions(&spec)

	if spec.StorageProfile != "" {
		if _, err := datacell.ParseStorageProfile(spec.StorageProfile); err != nil {
			return nil, errfmt.Errorf("spec %s: %w", specPath, err)
		}
	}

	// Set location reference (prefix: or abs:) so the backing file can be re-used where necessary
	spec.Location = pathRefForSpecFile(specPath)

	return &spec, nil
}

// ConvertInstanceVersionToBuilderVersion converts instance schema_version format to builder version format
// Instance format: InitialFieldVersion -> Builder format: "v1_0_0"
func ConvertInstanceVersionToBuilderVersion(instanceVersion string) string {
	if instanceVersion == emptyValue {
		return ""
	}
	// Remove leading 'v' if present, then add 'v' and replace dots with underscores
	version := strings.TrimPrefix(instanceVersion, "v")
	return "v" + strings.ReplaceAll(version, ".", "_")
}

// ConvertBuilderVersionToInstanceVersion converts builder version format to instance schema_version format
// Builder format: "v1_0_0" -> Instance format: InitialFieldVersion
func ConvertBuilderVersionToInstanceVersion(builderVersion string) string {
	if builderVersion == emptyValue {
		return ""
	}
	// Remove leading 'v', then replace underscores with dots
	version := strings.TrimPrefix(builderVersion, "v")
	return strings.ReplaceAll(version, "_", ".")
}

// LoadSpecByVersion loads a spec using the builder registry for the specified ontology and version
// This allows loading specs at specific versions (e.g., ledger InitialFieldVersion vs DefaultSchemaVersion)
// Instance version format: InitialFieldVersion (will be converted to builder format "v1_0_0")
// Returns a spec with resolved inheritance
// Requires SetBuilderRegistry to be called first (typically done by initialization code)
func (sl *SpecLoader) LoadSpecByVersion(ontology, instanceVersion string) (*Spec, error) {
	registry := sl.getBuilderRegistry()

	if registry == nil {
		return nil, errfmt.Errorf("builder registry not set - cannot load spec by version")
	}

	// Convert instance version format to builder version format
	builderVersion := ConvertInstanceVersionToBuilderVersion(instanceVersion)

	// Get builder from registry
	builder, err := registry.GetBuilder(ontology, builderVersion)
	if err != nil {
		// If exact version not found, try to use latest version as fallback
		// This handles cases where builder version (v1_0_0) doesn't match instance schema_version (2.0.0)
		latestVersion, latestErr := registry.GetLatestVersion(ontology)
		if latestErr == nil && latestVersion != builderVersion {
			builder, err = registry.GetBuilder(ontology, latestVersion)
			if err == nil {
				// Successfully loaded using latest version
				// Builder version and instance schema_version can differ (builder v1_0_0 can generate schema_version 2.0.0)
			}
		}
		if err != nil {
			return nil, errfmt.Errorf("failed to get builder for ontology %s version %s (tried %s and latest): %w", ontology, instanceVersion, builderVersion, err)
		}
	}

	// Build the spec from the builder
	spec := builder.Build()

	// Builders don't resolve inheritance, so we need to resolve it
	// Use the existing inheritance resolution logic
	// IMPORTANT: Clear cache before resolving inheritance to ensure parent specs are loaded fresh
	// This prevents stale cached parent specs from being used (e.g., with old validation patterns)
	if spec.Extends != emptyValue && spec.Extends != "null" {
		// OPTIMIZED: Lock-free invalidation using sync.Map.Delete
		parentShard := sl.getShard(spec.Extends)
		// Invalidate any cached parent specs that might be used during inheritance resolution
		// This ensures we get fresh specs with updated patterns from config/spec files
		parentShard.cache.Delete(spec.Extends)
	}

	visited := make(map[string]bool)
	resolvedSpec, err := sl.resolveSpecInheritance(spec, visited, 0)
	if err != nil {
		return nil, errfmt.Errorf("failed to resolve inheritance for ontology %s version %s: %w", ontology, instanceVersion, err)
	}

	return resolvedSpec, nil
}

// resolveSpecInheritance resolves inheritance for a spec (used by both YAML and builder loading)
func (sl *SpecLoader) resolveSpecInheritance(spec *Spec, visited map[string]bool, depth int) (*Spec, error) {
	if depth > maxSpecInheritanceDepth {
		return nil, errfmt.Errorf("spec inheritance depth exceeded %d (possible cycle or bug): %s", maxSpecInheritanceDepth, spec.Ontology)
	}
	// Check for circular inheritance
	if spec.Ontology != emptyValue {
		if visited[spec.Ontology] {
			return nil, errfmt.Errorf("circular inheritance detected: %s", spec.Ontology)
		}
		visited[spec.Ontology] = true
	}

	// Initialize resolved fields and traits with current spec's values
	resolvedSpec := &Spec{
		SchemaVersion:  spec.SchemaVersion,
		Ontology:       spec.Ontology,
		Extends:        spec.Extends,
		Visibility:     spec.Visibility,
		Description:    spec.Description,
		StorageProfile: spec.StorageProfile,
		Traits:         spec.Traits,
		ExcludeTraits:  spec.ExcludeTraits,
		Fields:         spec.Fields,
		ResolvedFields: make(map[string]any),
		ResolvedTraits: make([]string, len(spec.Traits)),
	}
	copy(resolvedSpec.ResolvedTraits, spec.Traits)

	// Copy fields to resolved fields
	if spec.Fields != nil {
		for k, v := range spec.Fields {
			resolvedSpec.ResolvedFields[k] = v
		}
	}

	// If this spec extends another, load and merge parent
	if spec.Extends != emptyValue && spec.Extends != "null" {
		// Try to load parent from builder registry first (if available)
		// Otherwise fall back to YAML file loading
		var parentSpec *Spec
		var err error

		// Try builder registry first - check if parent has builders registered
		registry := sl.getBuilderRegistry()

		if registry != nil {
			if versions := registry.GetVersions(spec.Extends); len(versions) > 0 {
				// Parent has builders - use latest version for inheritance resolution
				latestVersion, err := registry.GetLatestVersion(spec.Extends)
				if err == nil {
					builder, err := registry.GetBuilder(spec.Extends, latestVersion)
					if err == nil {
						parentSpecFromBuilder := builder.Build()
						// Recursively resolve parent's inheritance
						if resolvedParent, resolveErr := sl.resolveSpecInheritance(parentSpecFromBuilder, visited, depth+1); resolveErr == nil {
							parentSpec = resolvedParent
						}
					}
				}
			}
		}

		// Fall back to YAML file if builder didn't work
		if parentSpec == nil {
			parentFile := spec.Extends + ".yaml"
			// CRITICAL: Invalidate cached parent spec to ensure we load fresh (important for dynamic patterns)
			// This is essential for white-labeling - patterns must be read dynamically from config, not from cache
			parentShard := sl.getShard(spec.Extends)
			// OPTIMIZED: Lock-free invalidation using sync.Map.Delete
			parentShard.cache.Delete(spec.Extends)
			// Also invalidate any parent's parents (e.g., if base_object extends auditable, invalidate both)
			if parentFile == KindBaseObject+".yaml" {
				auditableShard := sl.getShard(KindAuditable)
				auditableShard.cache.Delete(KindAuditable)
			}
			parentSpec, err = sl.loadSpecWithInheritanceRecursive(parentFile, visited, "", depth+1)
			if err != nil {
				return nil, errfmt.Errorf("failed to load parent spec %s: %w", parentFile, err)
			}
		}

		// Merge parent fields (child overrides parent)
		for k, v := range parentSpec.ResolvedFields {
			if _, exists := resolvedSpec.ResolvedFields[k]; !exists {
				resolvedSpec.ResolvedFields[k] = v
			}
		}

		// Merge traits (union - no duplicates)
		traitMap := make(map[string]bool)
		for _, trait := range resolvedSpec.ResolvedTraits {
			traitMap[trait] = true
		}
		for _, trait := range parentSpec.ResolvedTraits {
			if !traitMap[trait] {
				resolvedSpec.ResolvedTraits = append(resolvedSpec.ResolvedTraits, trait)
				traitMap[trait] = true
			}
		}
		// Merge exclude_traits (union - no duplicates)
		excludedMap := make(map[string]bool)
		for _, t := range resolvedSpec.ExcludeTraits {
			excludedMap[t] = true
		}
		for _, t := range parentSpec.ExcludeTraits {
			if !excludedMap[t] {
				resolvedSpec.ExcludeTraits = append(resolvedSpec.ExcludeTraits, t)
				excludedMap[t] = true
			}
		}

		if resolvedSpec.StorageProfile == "" && parentSpec.StorageProfile != "" {
			resolvedSpec.StorageProfile = parentSpec.StorageProfile
		}
	}
	applyTraitExclusions(resolvedSpec)

	if resolvedSpec.StorageProfile != "" {
		if _, err := datacell.ParseStorageProfile(resolvedSpec.StorageProfile); err != nil {
			return nil, errfmt.Errorf("spec %s: %w", resolvedSpec.Ontology, err)
		}
	}

	return resolvedSpec, nil
}

func applyTraitExclusions(spec *Spec) {
	if spec == nil || len(spec.ExcludeTraits) == 0 || len(spec.ResolvedTraits) == 0 {
		return
	}
	excluded := make(map[string]bool, len(spec.ExcludeTraits))
	for _, t := range spec.ExcludeTraits {
		if t != emptyValue {
			excluded[t] = true
		}
	}
	filtered := make([]string, 0, len(spec.ResolvedTraits))
	for _, t := range spec.ResolvedTraits {
		if !excluded[t] {
			filtered = append(filtered, t)
		}
	}
	spec.ResolvedTraits = filtered
}

// getOntologyFromFile extracts ontology from a spec file without full parsing
// Results are cached to avoid re-reading files
func (sl *SpecLoader) getOntologyFromFile(filePath string) string {
	// Use file path hash to determine shard (different from ontology-based sharding)
	// This ensures ontology cache lookups don't contend with spec cache lookups
	hash := 0
	for _, c := range filePath {
		hash = hash*31 + int(c)
	}
	shardIndex := hash & (sl.shardCount - 1)
	shard := sl.shards[shardIndex]

	// OPTIMIZED: Lock-free cache read using sync.Map (hot path)
	cachedVal, exists := shard.ontologyCache.Load(filePath)
	if exists {
		return cachedVal.(string)
	}

	// Read and parse file (I/O outside lock)
	data, mtime, err := sl.readSpecFile(context.Background(), filePath)
	if err != nil {
		return ""
	}

	var partial struct {
		Ontology string `yaml:"ontology"`
	}
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return ""
	}

	// OPTIMIZED: Lock-free double-checked pattern using sync.Map
	// Another goroutine might have loaded it while we were reading
	cachedAfterReadVal, existsAfterRead := shard.ontologyCache.Load(filePath)
	if existsAfterRead {
		return cachedAfterReadVal.(string)
	}

	// Cache the ontology and raw file content so loadSpecWithInheritanceRecursive can use it
	// (avoids second ReadFile for the same path - hot path optimization per OBJECT_OPERATIONS_PERFORMANCE.md)
	// Queue cache updates instead of blocking on Store()
	sl.queueCacheUpdate(shard, CacheUpdateOntology, filePath, partial.Ontology)
	sl.queueCacheUpdate(shard, CacheUpdateFileContent, filePath, &fileContentEntry{data: data, mtime: mtime})
	return partial.Ontology
}

var (
	globalSpecLoader *SpecLoader
	specLoaderOnce   sync.Once
)

// GetGlobalSpecLoader returns the global spec loader instance
// This ensures caches are shared across all validation operations (sync and async)
func GetGlobalSpecLoader() *SpecLoader {
	specLoaderOnce.Do(func() {
		globalSpecLoader = NewSpecLoader("")
	})
	return globalSpecLoader
}

// DiscoverOntologies returns all available ontologies in the specs directory.
func (sl *SpecLoader) DiscoverOntologies() ([]string, error) {
	entries, err := os.ReadDir(sl.specsDir)
	if err != nil {
		return nil, errfmt.Newf("failed to read specs directory").Wrap(err)
	}

	var ontologies []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".yaml") {
			ontologies = append(ontologies, strings.TrimSuffix(entry.Name(), ".yaml"))
		}
	}
	sort.Strings(ontologies)
	return ontologies, nil
}

// LoadSpec loads a single spec by ontology (without inheritance).
func (sl *SpecLoader) LoadSpec(ontology string) (*Spec, error) {
	specFile := ontology + ".yaml"
	data, _, err := sl.readSpecFile(context.Background(), filepath.Join(sl.specsDir, specFile))
	if err != nil {
		return nil, err
	}
	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Newf("failed to parse spec").Wrap(err)
	}
	if spec.Ontology == "" {
		spec.Ontology = ontology
	}
	return &spec, nil
}

// GetMetrics returns the metrics snapshot for this spec loader
// Useful for monitoring lock contention and timeout frequency
func (sl *SpecLoader) GetMetrics() SpecLoaderMetricsSnapshot {
	return sl.metrics.GetSnapshot()
}

// SpecCacheRevision returns a monotonic counter incremented whenever this loader clears or
// invalidates its in-memory spec cache (ClearCache, InvalidateSpec, InvalidateSpecByFile).
// It is the loader-local contribution to a unified spec-plane snapshot story; compare before/after
// regenerating spec-derived artifacts (spec index, dependency graph walks, kind lists).
func (sl *SpecLoader) SpecCacheRevision() uint64 {
	if sl == nil {
		return 0
	}
	return sl.cacheRevision.Load()
}

func (sl *SpecLoader) bumpSpecCacheRevision() {
	sl.cacheRevision.Add(1)
}

// ClearCache clears the spec cache, forcing reload of all specs on next access
// This is useful when specs are updated and you want to ensure fresh validation
// OPTIMIZED: Uses sync.Map.Range for lock-free iteration, then Delete for each entry
// For better performance, use InvalidateSpec() to invalidate specific entries.
func (sl *SpecLoader) ClearCache() {
	// Clear all shards (lock-free iteration using sync.Map.Range)
	for _, shard := range sl.shards {
		// Clear spec cache (lock-free)
		shard.cache.Range(func(key, value any) bool {
			shard.cache.Delete(key)
			return true
		})
		// Clear ontology cache (lock-free)
		shard.ontologyCache.Range(func(key, value any) bool {
			shard.ontologyCache.Delete(key)
			return true
		})
		// Clear path cache (avoids stale path-based hits after full clear)
		shard.pathCache.Range(func(key, value any) bool {
			shard.pathCache.Delete(key)
			return true
		})
		// Clear file content cache (raw bytes per path)
		shard.fileContentCache.Range(func(key, value any) bool {
			shard.fileContentCache.Delete(key)
			return true
		})
	}
	sl.bumpSpecCacheRevision()
}

// InvalidateSpec invalidates a specific spec entry by ontology
// OPTIMIZED: Lock-free deletion using sync.Map.Delete
// Use this instead of ClearCache() when you know which spec changed
func (sl *SpecLoader) InvalidateSpec(ontology string) {
	shard := sl.getShard(ontology)
	// Lock-free deletion (sync.Map.Delete is thread-safe)
	shard.cache.Delete(ontology)
	sl.bumpSpecCacheRevision()
}

// InvalidateSpecByFile invalidates a spec entry by file path
// This is useful when you know a file changed but not the ontology
func (sl *SpecLoader) InvalidateSpecByFile(filePath string) {
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(sl.specsDir, filePath)
	}
	if abs, err := filepath.Abs(absPath); err == nil {
		absPath = abs
	}
	pathShard := sl.getShardByPath(absPath)
	// Clear path-based caches so next load re-reads the file
	if ontologyVal, ok := pathShard.ontologyCache.Load(absPath); ok {
		ontology := ontologyVal.(string)
		shard := sl.getShard(ontology)
		shard.cache.Delete(ontology)
	}
	pathShard.pathCache.Delete(absPath)
	pathShard.ontologyCache.Delete(absPath)
	pathShard.fileContentCache.Delete(absPath)
	sl.bumpSpecCacheRevision()
}

// FindSpecsDir returns the object specs directory (docs/architecture/_internal/object_specs or equivalent).
// Used by SpecLoader and by scheduler cache prewarm to discover spec files.
func FindSpecsDir() string {
	return findSpecsDir()
}

// baseObjectSpecName is the spec file that must exist for a valid specs dir (avoids using a bare docs/architecture/_internal/object_specs from a sibling .zqk project).
const baseObjectSpecName = "base_object.yaml"

// normalizeSpecsDirIfProjectRoot returns docs/architecture/_internal/object_specs under root
// when root does not contain base_object.yaml but that nested path does. Otherwise returns
// specsDir unchanged (already the object_specs dir, or unknown layout).
func normalizeSpecsDirIfProjectRoot(specsDir string) string {
	if specsDir == emptyValue {
		return ""
	}
	clean := filepath.Clean(specsDir)
	if specsDirHasBaseSpec(clean) {
		return clean
	}
	nested := filepath.Join(clean, paths.ProcessInternalObjectSpecsDir)
	if specsDirHasBaseSpec(nested) {
		return nested
	}
	return clean
}

// specsDirHasBaseSpec returns true if the directory contains base_object.yaml (so it's a real spec tree, not an empty clone).
func specsDirHasBaseSpec(specsDir string) bool {
	_, err := os.Stat(filepath.Join(specsDir, baseObjectSpecName))
	return err == nil
}

// findSpecsDir attempts to find the object specs directory
func findSpecsDir() string {
	// When running in tests, use test root only if it has a valid spec tree (base_object.yaml).
	// SetupTestEnvironment creates an empty docs/architecture/_internal/object_specs; without
	// base_object.yaml LoadFields() would fail. Fall through to repo specs when test root
	// has no base_object so tests that don't copy specs still pass (OBJECT_OPERATIONS_PERFORMANCE.md).
	if testRoot := os.Getenv(zqkenv.TestRoot()); testRoot != emptyValue {
		specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
		if info, err := os.Stat(specsDir); err == nil && info.IsDir() && specsDirHasBaseSpec(specsDir) {
			return specsDir
		}
	}
	// Try common locations
	possiblePaths := []string{
		paths.ProcessInternalObjectSpecsDir,
		filepath.Join("..", paths.ProcessInternalObjectSpecsDir),
		filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir),
	}

	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	for _, path := range possiblePaths {
		absPath := filepath.Join(wd, path)
		if info, err := os.Stat(absPath); err == nil && info.IsDir() && specsDirHasBaseSpec(absPath) {
			return absPath
		}
	}

	// Walk up directory tree; require base_object.yaml so we use the real repo spec tree (not an empty docs/process from a nested .zqk).
	dir := wd
	for {
		potentialPath := filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)
		if info, err := os.Stat(potentialPath); err == nil && info.IsDir() && specsDirHasBaseSpec(potentialPath) {
			return potentialPath
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}
