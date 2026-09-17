package validation

import (
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/loader"
	"github.com/lanceman/zqk/pkg/logging"
)

// Constants and deprecated helpers are now in id_validator_constants.go

// IDPatternConfig represents ID pattern configuration for an object kind
type IDPatternConfig struct {
	Kind     string
	Prefixes []string // Valid prefixes (e.g., ["PRI-", "PRIO-"])
	Pattern  string   // Optional regex pattern (if not provided, uses prefix-based pattern)
	Template string   // Optional template (e.g., "BLI-{sequence}")
}

// IDValidator validates object IDs against configured patterns
type IDValidator struct {
	patterns           map[string]*IDPatternConfig
	mu                 sync.RWMutex
	specsDir           string
	graphConn          provider.GraphConnection // Optional graph connection for graph-based specs
	useGraph           bool                     // Whether to load from graph
	patternsRunner     *loader.Runner           // Retryable loader (atomics + channel + timeout); lazily inited
	patternsRunnerOnce sync.Once
	eventLogger        *logging.EventLogger // Cached event logger to avoid creating new ones (initialized lazily)
	loggerMu           sync.RWMutex         // Protects logger initialization
	idPrefixesConfig   *IDPrefixesConfig    // Optional: injected config (for test isolation), nil uses global
	pathsConfig        *PathsConfig         // Optional: injected paths config (for test isolation), nil uses global
}

var (
	globalValidator *IDValidator
	validatorOnce   sync.Once
)

// GetIDValidator returns the global ID validator instance
func GetIDValidator() *IDValidator {
	validatorOnce.Do(func() {
		globalValidator = NewIDValidator("")
	})
	return globalValidator
}

// NewIDValidator creates a new ID validator
// If specsDir is empty, it will attempt to discover the specs directory
// Uses global configs (GetGlobalIDPrefixesConfig, GetGlobalPathsConfig)
func NewIDValidator(specsDir string) *IDValidator {
	if specsDir == emptyValue {
		specsDir = discoverSpecsDir()
	}
	return &IDValidator{
		patterns:         make(map[string]*IDPatternConfig),
		specsDir:         specsDir,
		useGraph:         false,
		idPrefixesConfig: nil, // Use global
		pathsConfig:      nil, // Use global
	}
}

// NewIDValidatorWithConfigs creates a new ID validator with injected configs
// This enables test isolation by using test-specific configs instead of global singletons
// If specsDir is empty, it will attempt to discover the specs directory
// If idPrefixesConfig or pathsConfig are nil, falls back to global configs
func NewIDValidatorWithConfigs(specsDir string, idPrefixesConfig *IDPrefixesConfig, pathsConfig *PathsConfig) *IDValidator {
	if specsDir == emptyValue {
		// Use injected pathsConfig if available, otherwise fall back to global discovery
		if pathsConfig != nil {
			if path := pathsConfig.FindPath("object_specs"); path != emptyValue {
				specsDir = path
			} else {
				specsDir = discoverSpecsDir()
			}
		} else {
			specsDir = discoverSpecsDir()
		}
	}
	return &IDValidator{
		patterns:         make(map[string]*IDPatternConfig),
		specsDir:         specsDir,
		useGraph:         false,
		idPrefixesConfig: idPrefixesConfig, // Use injected config
		pathsConfig:      pathsConfig,      // Use injected config
	}
}

// NewIDValidatorWithGraph creates a new ID validator with graph backend support
// Uses global configs (GetGlobalIDPrefixesConfig, GetGlobalPathsConfig)
func NewIDValidatorWithGraph(specsDir string, graphConn provider.GraphConnection) *IDValidator {
	if specsDir == emptyValue {
		specsDir = discoverSpecsDir()
	}
	return &IDValidator{
		patterns:         make(map[string]*IDPatternConfig),
		specsDir:         specsDir,
		graphConn:        graphConn,
		useGraph:         graphConn != nil,
		idPrefixesConfig: nil, // Use global
		pathsConfig:      nil, // Use global
	}
}

// NewIDValidatorWithGraphAndConfigs creates a new ID validator with graph backend support and injected configs
// This enables test isolation by using test-specific configs instead of global singletons
// If specsDir is empty, it will attempt to discover the specs directory
// If idPrefixesConfig or pathsConfig are nil, falls back to global configs
func NewIDValidatorWithGraphAndConfigs(specsDir string, graphConn provider.GraphConnection, idPrefixesConfig *IDPrefixesConfig, pathsConfig *PathsConfig) *IDValidator {
	if specsDir == emptyValue {
		// Use injected pathsConfig if available, otherwise fall back to global discovery
		if pathsConfig != nil {
			if path := pathsConfig.FindPath("object_specs"); path != emptyValue {
				specsDir = path
			} else {
				specsDir = discoverSpecsDir()
			}
		} else {
			specsDir = discoverSpecsDir()
		}
	}
	return &IDValidator{
		patterns:         make(map[string]*IDPatternConfig),
		specsDir:         specsDir,
		graphConn:        graphConn,
		useGraph:         graphConn != nil,
		idPrefixesConfig: idPrefixesConfig, // Use injected config
		pathsConfig:      pathsConfig,      // Use injected config
	}
}

// getIDPrefixesConfig returns the instance config if set, otherwise falls back to global
// This enables dependency injection for test isolation while maintaining backward compatibility
func (v *IDValidator) getIDPrefixesConfig() *IDPrefixesConfig {
	if v.idPrefixesConfig != nil {
		return v.idPrefixesConfig
	}
	return GetGlobalIDPrefixesConfig()
}

// getPathsConfig returns the instance config if set, otherwise falls back to global
// This enables dependency injection for test isolation while maintaining backward compatibility
func (v *IDValidator) getPathsConfig() *PathsConfig {
	if v.pathsConfig != nil {
		return v.pathsConfig
	}
	return GetGlobalPathsConfig()
}

// SetGraphConnection sets the graph connection for graph-based spec loading
func (v *IDValidator) SetGraphConnection(conn provider.GraphConnection) {
	if err := concurrency.RunInLockWithLogger(
		&v.mu,
		LockNameIdValidatorSetGraphConnection,
		lockLoggerSystem(),
		func() error {
			v.graphConn = conn
			v.useGraph = conn != nil
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// hasGraphBackend returns true if graph backend is configured and available
			// Must be called with lock held (read or write)
			//
			//nolint:unused // Called conditionally in loadPatternsUnlocked - may not be detected by static analysis
			ProfileSystem))).Error(ConstMagic88972993, err).Log()
	}
}

func (v *IDValidator) hasGraphBackend() bool {
	return v.useGraph && v.graphConn != nil
}

// hasFileBackend returns true if file-based backend is configured
// Must be called with lock held (read or write)
//
//nolint:unused // Called conditionally in loadPatternsUnlocked - may not be detected by static analysis
func (v *IDValidator) hasFileBackend() bool {
	return v.specsDir != emptyValue
}

// Loading logic is now in id_validator_loading.go
// Validation logic is now in id_validator_validation.go
// Path discovery helpers are now in id_validator_paths.go
// Graph loading functions are now in id_validator_loading.go

// SetSpecsDirForTest sets the specs directory for test environments.
func (v *IDValidator) SetSpecsDirForTest(specsDir string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.specsDir = specsDir
}

// InjectPatternForTest injects a pattern config directly for unit testing
func (v *IDValidator) InjectPatternForTest(kind string, pattern string, prefixes ...string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.patterns == nil {
		v.patterns = make(map[string]*IDPatternConfig)
	}
	v.patterns[kind] = &IDPatternConfig{
		Kind:     kind,
		Pattern:  pattern,
		Prefixes: prefixes,
	}
}

// ResetGlobalIDValidatorForTest resets the global validator singleton (for unit tests).
func ResetGlobalIDValidatorForTest() {
	globalValidator = nil
	validatorOnce = sync.Once{}
}
