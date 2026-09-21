package objects

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/loader"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// kindSynonymsObjectDirectory is the on-disk folder for KindSynonym (plural name).
const kindSynonymsObjectDirectory = "kind_synonyms"

// DynamicKindMapper discovers and caches kind-to-directory mappings dynamically.
// EnsureReady(ctx) uses the component loader pattern (pkg/loader.Runner) for deterministic init, timeouts, and telemetry.
type DynamicKindMapper struct {
	processDir      string
	specsDir        string
	specLoader      *SpecLoader
	config          *KindMappingsConfig // Configuration for mappings
	cache           map[string]string   // kind -> directory
	reverseCache    map[string]string   // directory -> kind
	mu              sync.RWMutex
	initialized     bool
	initOnce        sync.Once
	readyRunner     *loader.Runner
	readyRunnerOnce sync.Once
}

var (
	globalKindMapper     *DynamicKindMapper
	globalKindMapperOnce sync.Once
	globalKindMapperMu   sync.Mutex
)

// GetGlobalKindMapper returns the singleton instance of DynamicKindMapper
func GetGlobalKindMapper() *DynamicKindMapper {
	globalKindMapperMu.Lock()
	defer globalKindMapperMu.Unlock()
	globalKindMapperOnce.Do(func() {
		globalKindMapper = NewDynamicKindMapper("", "")
	})
	return globalKindMapper
}

// ResetGlobalKindMapperForTesting resets the global kind mapper singleton.
// This is required for test isolation when tests modify ZQK_TEST_ROOT.
func ResetGlobalKindMapperForTesting() {
	globalKindMapperMu.Lock()
	defer globalKindMapperMu.Unlock()
	globalKindMapperOnce = sync.Once{}
	globalKindMapper = nil
}

// NewDynamicKindMapper creates a new dynamic kind mapper
func NewDynamicKindMapper(processDir, specsDir string) *DynamicKindMapper {
	if processDir == emptyValue {
		processDir = findProcessDir()
	}
	if specsDir == emptyValue {
		specsDir = findSpecsDir()
	}

	return &DynamicKindMapper{
		processDir:   processDir,
		specsDir:     specsDir,
		specLoader:   NewSpecLoader(specsDir),
		config:       GetGlobalKindMappingsConfig(),
		cache:        make(map[string]string),
		reverseCache: make(map[string]string),
		initialized:  false,
	}
}

// Initialize discovers and caches all kind-to-directory mappings
func (dkm *DynamicKindMapper) Initialize() error {
	start := time.Now()
	metrics := GetKindMappingsMetrics()

	var initErr error
	dkm.initOnce.Do(func() {
		initErr = dkm.doInitialize(start, metrics)
	})
	return initErr
}

func (dkm *DynamicKindMapper) doInitialize(start time.Time, metrics *KindMappingsMetrics) error {
	var alreadyInitialized bool
	err := concurrency.RunInLockWithLogger(
		&dkm.mu, LockNameKindMapperInitializeCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			alreadyInitialized = dkm.initialized
			return nil
		},
	)
	if err != nil {
		return err
	}

	if alreadyInitialized {
		return nil
	}

	var entries []fileutil.DirEntry
	if dkm.processDir != emptyValue {
		e, err := fileutil.ReadDir(dkm.processDir)
		if err != nil {
			if !fileutil.IsNotExist(err) {
				err = errfmt.Newf("failed to read process directory").Wrap(err)
				metrics.RecordInitialization(time.Since(start), err)
				return err
			}
		} else {
			entries = e
		}
	}

	// Build mappings outside lock (copy-out/process pattern)
	mappings := make(map[string]string)
	reverseMappings := make(map[string]string)

	// For each directory, try to find corresponding spec and determine kind
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		metrics.RecordDirectoryScan()

		// Skip system directories using config
		if dkm.config.ShouldSkipDirectory(dirName) {
			continue
		}

		// Try to find a spec file that might correspond to this directory
		// Strategy: look for spec files and match by ontology or by naming convention
		kind := dkm.discoverKindForDirectory(dirName)
		if kind != emptyValue {
			mappings[kind] = dirName
			reverseMappings[dirName] = kind
			metrics.RecordMappingDiscovered()
		}
	}

	// Also scan specs directory to find kinds that might not have directories yet
	// This ensures we have mappings for all known kinds, even if directories don't exist
	if dkm.specsDir != emptyValue {
		ontologies, err := dkm.specLoader.DiscoverOntologies()
		if err == nil {
			for _, baseName := range ontologies {
				metrics.RecordSpecScan()

				// Skip base specs using config
				if dkm.config.ShouldSkipSpec(baseName) || dkm.config.ShouldSkipSpec(baseName+".yaml") {
					continue
				}

				// Load spec to get ontology
				spec, err := dkm.specLoader.LoadSpecWithInheritance(baseName + ".yaml")
				if err != nil {
					continue
				}

				kind := spec.Ontology
				if kind == emptyValue {
					kind = baseName
				}

				// If we don't have a mapping for this kind yet, check explicit mappings first
				if _, exists := mappings[kind]; !exists {
					var dirName string

					// Check config mappings first (explicit mappings take precedence)
					if dkm.config != nil {
						if dir := dkm.config.GetDirectoryFromKind(kind); dir != emptyValue && dir != kind {
							dirName = dir
						}
					}

					// Check legacy mappings if no config mapping found
					if dirName == emptyValue {
						if dir, ok := legacyKindToDirectory[kind]; ok {
							dirName = dir
						}
					}

					// Only use inference as last resort (for truly unknown kinds)
					if dirName == emptyValue {
						dirName = dkm.inferDirectoryName(kind, baseName)
					}

					// FAIL-CLOSED: Only register if dirName is explicit or if it exists in the filesystem.
					if dirName != emptyValue {
						// Check if directory actually exists or is explicitly on-demand in config
						potentialPath := filepath.Join(dkm.processDir, dirName)
						dirExists := false
						if info, err := fileutil.Stat(potentialPath); err == nil && info.IsDir() {
							dirExists = true
						}

						isExplicitConfig := false
						if dkm.config != nil && (dkm.config.GetDirectoryFromKind(kind) != emptyValue || dkm.config.IsOnDemandKind(kind)) {
							isExplicitConfig = true
						}

						if dirExists || isExplicitConfig {
							mappings[kind] = dirName
							reverseMappings[dirName] = kind
							metrics.RecordMappingDiscovered()
						}
					}
				}
			}
		}
	}

	// Copy-in: Update cache with lock
	err = concurrency.RunInLockWithLogger(
		&dkm.mu, LockNameKindMapperInitializeUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Double-check after re-acquiring lock
			if dkm.initialized {
				return nil
			}
			// Update cache with discovered mappings
			for kind, dir := range mappings {
				dkm.cache[kind] = dir
			}
			for dir, kind := range reverseMappings {
				dkm.reverseCache[dir] = kind
			}
			dkm.initialized = true
			return nil
		},
	)
	if err != nil {
		return err
	}

	metrics.RecordInitialization(time.Since(start), nil)
	return nil
}

// getReadyRunner returns the shared loader.Runner for "ensure kind mapper ready" (lazily created).
func (dkm *DynamicKindMapper) getReadyRunner() *loader.Runner {
	dkm.readyRunnerOnce.Do(func() {
		dkm.readyRunner = loader.NewRunner("kind_mapper", func(ctx context.Context) error {
			return dkm.Initialize()
		})
	})
	return dkm.readyRunner
}

// EnsureReady ensures the kind mapper is initialized (discover and cache kind-to-directory mappings).
// Uses the component loader pattern (pkg/loader.Runner) so concurrent callers wait with timeout and telemetry is consistent.
func (dkm *DynamicKindMapper) EnsureReady(ctx context.Context) error {
	return dkm.getReadyRunner().Load(ctx)
}

// discoverKindForDirectory tries to discover the object kind for a given directory.
// Only explicit configuration and legacy directory mappings are supported (no spec-filename inference).
// TRACK: TDE-CEF-KIND-MAP-SPEC-DISCOVERY-001 / BLI-TDE-KIND-MAP-SPEC-DISCOVERY-001
func (dkm *DynamicKindMapper) discoverKindForDirectory(dirName string) string {
	// Strategy 0: Check config mappings first (including multi-kind defaults)
	if dkm.config != nil {
		// Use GetKindFromDirectory which handles backend-specific configs
		if kind := dkm.config.GetKindFromDirectory(dirName); kind != emptyValue && kind != dirName {
			return kind
		}
	}

	// Strategy 0.5: Check legacy mappings (for backward compatibility)
	if kind, ok := legacyDirectoryToKind[dirName]; ok {
		return kind
	}

	return ""
}

// inferDirectoryName infers a directory name from a kind name
func (dkm *DynamicKindMapper) inferDirectoryName(kind, _ string) string {
	// Use config-based inference first (includes explicit mappings and pattern rules)
	if dkm.config != nil {
		if dir := dkm.config.GetDirectoryFromKind(kind); dir != emptyValue && dir != kind {
			return dir
		}
	}

	// Fallback: Only use simple pattern-based inference if config doesn't have rules
	// These are minimal fallbacks for cases where config might not be available
	// All explicit mappings should be in the config file, not here

	// Pattern: Remove _item suffix (e.g., backlog_item -> backlog)
	if strings.HasSuffix(kind, "_item") {
		return strings.TrimSuffix(kind, "_item")
	}

	// Pattern: Metric types go to metrics directory
	if strings.HasSuffix(kind, "_metric") {
		return "metrics"
	}

	// Default: try pluralizing (simple fallback)
	if !strings.HasSuffix(kind, "s") {
		if strings.HasSuffix(kind, "y") && kind != "boy" && kind != "toy" && kind != "way" && kind != "day" {
			return strings.TrimSuffix(kind, "y") + "ies"
		}
		return kind + "s"
	}

	return kind
}

// GetDirectoryFromKind returns the directory name for a given object kind
func (dkm *DynamicKindMapper) GetDirectoryFromKind(kind string) string {
	// Check config mappings FIRST (before initialization) - prioritize explicit mappings
	// This avoids deadlocks when Initialize() is holding a write lock
	if dkm.config != nil {
		if dir := dkm.config.GetDirectoryFromKind(kind); dir != emptyValue && dir != kind {
			// Always return explicit config mappings - directories will be created on-demand
			// This prevents wrong directories (e.g., audit_events) from being used instead of correct ones (e.g., audit)
			return dir
		}
	}

	// Check legacy mappings SECOND (before initialization) - for backward compatibility
	// Legacy mappings are explicit and should always be used
	if dir, ok := legacyKindToDirectory[kind]; ok {
		return dir
	}

	// Only now check if we need to initialize (for unknown kinds)
	var initialized bool
	_ = concurrency.RunInRLockWithLogger(
		&dkm.mu, LockNameKindMapperGetDirCheckInit, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			initialized = dkm.initialized
			return nil
		},
	)

	if !initialized {
		if err := dkm.Initialize(); err != nil {
			return ""
		}
	}

	var dir string
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&dkm.mu, LockNameKindMapperGetDirLookup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			dir, ok = dkm.cache[kind]
			exists = ok
			return nil
		},
	)

	if exists {
		return dir
	}

	// Fallback: use inference if directory exists or for known patterns
	// This allows kinds to work even if not yet discovered
	inferredDir := dkm.inferDirectoryName(kind, "")
	if inferredDir != emptyValue {
		// Verify directory exists (for test environments)
		potentialPath := filepath.Join(dkm.processDir, inferredDir)
		if _, err := fileutil.Stat(potentialPath); err == nil {
			return inferredDir
		}
		// Also allow if it's an on-demand kind (directories will be created on-demand)
		if dkm.config != nil && dkm.config.IsOnDemandKind(kind) {
			return inferredDir
		}
	}

	// For truly unknown kinds, return empty string
	return ""
}

// GetKindFromDirectory returns the object kind for a given directory name
func (dkm *DynamicKindMapper) GetKindFromDirectory(dirName string) string {
	var initialized bool
	_ = concurrency.RunInRLockWithLogger(
		&dkm.mu, LockNameKindMapperGetKindCheckInit, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			initialized = dkm.initialized
			return nil
		},
	)

	if !initialized {
		if err := dkm.Initialize(); err != nil {
			return ""
		}
	}

	// Check config first (before cache) for multi-kind directories
	// This ensures config takes precedence over discovered mappings
	if dkm.config != nil {
		// Use GetKindFromDirectory which handles backend-specific configs
		if kind := dkm.config.GetKindFromDirectory(dirName); kind != emptyValue && kind != dirName {
			return kind
		}
	}

	var cachedKind string
	var hasCache bool
	_ = concurrency.RunInRLockWithLogger(
		&dkm.mu, LockNameKindMapperGetKindLookup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			cachedKind, ok = dkm.reverseCache[dirName]
			hasCache = ok
			return nil
		},
	)

	if hasCache {
		return cachedKind
	}

	// Fallback: try to discover
	return dkm.discoverKindForDirectory(dirName)
}

// GetAllKinds returns all known object kinds
func (dkm *DynamicKindMapper) GetAllKinds() []string {
	var initialized bool
	_ = concurrency.RunInRLockWithLogger(
		&dkm.mu, LockNameKindMappingsDynamicGetAllCheckInit, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			initialized = dkm.initialized
			return nil
		},
	)

	if !initialized {
		if err := dkm.Initialize(); err != nil {
			return []string{}
		}
	}

	var kinds []string
	_ = concurrency.RunInRLockWithLogger(
		&dkm.mu, LockNameKindMappingsDynamicGetAll, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			kinds = make([]string, 0, len(dkm.cache))
			for kind := range dkm.cache {
				kinds = append(kinds, kind)
			}
			return nil
		},
	)
	return kinds
}

// Reload clears the cache and reinitializes
func (dkm *DynamicKindMapper) Reload() error {
	err := concurrency.RunInLockWithLogger(
		&dkm.mu, LockNameKindMapperReload, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			dkm.cache = make(map[string]string)
			dkm.reverseCache = make(map[string]string)
			dkm.initialized = false
			return nil
		},
	)
	if err != nil {
		return err
	}

	return dkm.Initialize()
}

// SetDirectories sets the process and specs directories and resets initialization state.
// Call this when storage is created with a known project root so the global mapper
// uses the correct paths instead of findProcessDir()/findSpecsDir() (which use Getwd()).
// Next call to Initialize() will use the new directories. Thread-safe.
func (dkm *DynamicKindMapper) SetDirectories(processDir, specsDir string) {
	_ = concurrency.RunInLockWithLogger(
		&dkm.mu, LockNameKindMapperSetDirectories, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			dkm.processDir = processDir
			dkm.specsDir = specsDir
			if specsDir != emptyValue {
				dkm.specLoader = NewSpecLoader(specsDir)
			}
			dkm.cache = make(map[string]string)
			dkm.reverseCache = make(map[string]string)
			dkm.initialized = false
			return nil
		},
	)
}

// GetDirectories returns the process and specs directories used by the mapper. Thread-safe.
func (dkm *DynamicKindMapper) GetDirectories() (string, string) {
	var processDir, specsDir string
	_ = concurrency.RunInRLockWithLogger(
		&dkm.mu, locknames.LockNameKindMapperGetDirectories, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			processDir = dkm.processDir
			specsDir = dkm.specsDir
			return nil
		},
	)
	return processDir, specsDir
}

// findProcessDir attempts to find the process directory
func findProcessDir() string {
	if testRoot := zqkenv.TestRoot().Get(); testRoot != emptyValue {
		processDir := datacell.ProcessPrimaryDir(testRoot)
		if info, err := fileutil.Stat(processDir); err == nil && info.IsDir() {
			return processDir
		}
	}

	// Try common locations
	possiblePaths := []string{
		datacell.ProcessPrimaryDir("."),
		datacell.ProcessPrimaryDir(".."),
		datacell.ProcessPrimaryDir(filepath.Join("..", "..")),
	}

	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}

	for _, path := range possiblePaths {
		absPath := filepath.Join(wd, path)
		if info, err := fileutil.Stat(absPath); err == nil && info.IsDir() {
			return absPath
		}
	}

	// Walk up directory tree
	dir := wd
	for {
		potentialPath := datacell.ProcessPrimaryDir(dir)
		if _, err := fileutil.Stat(potentialPath); err == nil {
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
