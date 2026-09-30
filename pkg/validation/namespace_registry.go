package validation

import (
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var specNamespaceOverlays stampmemo.Table[map[string]string] // keyed by specsDir; stamp is top-level YAML

// NamespaceRegistry maps object kinds to their default namespaces
type NamespaceRegistry struct {
	kindToNamespace       map[string]string   // kind -> namespace_id
	subordinateNamespaces map[string][]string // parent -> []subordinate
	mu                    sync.RWMutex
	loadMu                sync.Mutex
	specsDir              string
	loaded                bool
}

var (
	globalNamespaceRegistry *NamespaceRegistry
	namespaceRegistryOnce   sync.Once
)

// GetNamespaceRegistry returns the global namespace registry instance
func GetNamespaceRegistry() *NamespaceRegistry {
	namespaceRegistryOnce.Do(func() {
		globalNamespaceRegistry = NewNamespaceRegistry("")
	})
	return globalNamespaceRegistry
}

// NewNamespaceRegistry creates a new namespace registry
func NewNamespaceRegistry(specsDir string) *NamespaceRegistry {
	if specsDir == emptyValue {
		// Use the same discovery logic as IDValidator
		specsDir = discoverSpecsDirForRegistry()
	}
	return &NamespaceRegistry{
		kindToNamespace:       make(map[string]string),
		subordinateNamespaces: make(map[string][]string),
		specsDir:              specsDir,
		loaded:                false,
	}
}

// discoverSpecsDirForRegistry discovers the object specs directory
// Uses paths configuration instead of hardcoded paths
func discoverSpecsDirForRegistry() string {
	config := GetGlobalPathsConfig()
	if config != nil {
		if path := config.FindPath("object_specs"); path != emptyValue {
			return path
		}
	}

	// Fallback to default if config not available
	return findPathWithDefaults(paths.ProcessInternalObjectSpecsDir, true)
}

// LoadNamespaces loads namespace mappings from object specs
// Looks for namespace_id field in specs, or infers from ontology/kind
// This method releases the lock before doing I/O to avoid blocking readers
// LoadNamespaces loads namespace mappings from specs directory
//
//nolint:gocyclo // Function orchestrates namespace loading phases; complexity reduced via helper methods
func (nr *NamespaceRegistry) LoadNamespaces() error {
	// Check if already loaded
	if nr.isLoaded() {
		return nil
	}

	nr.loadMu.Lock()
	defer nr.loadMu.Unlock()

	// Double-check after acquiring lock
	if nr.isLoaded() {
		return nil
	}

	// Get specs directory (releases lock before I/O)
	specsDir := nr.getSpecsDirForIO()

	// Load namespace mappings
	newMappings := nr.loadNamespaceMappings(specsDir)

	// Load subordinate namespace relationships
	subordinateMap := nr.loadSubordinateNamespaces()

	// Update registry (acquires lock again)
	return nr.updateRegistry(newMappings, subordinateMap)
}

// isLoaded checks if namespaces are already loaded
func (nr *NamespaceRegistry) isLoaded() bool {
	var loaded bool
	if err := concurrency.RunInLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryIsLoaded,
		lockLoggerSystem(),
		func() error {
			loaded = nr.loaded
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// getSpecsDirForIO gets specs directory and releases lock before I/O
			Error("lock failed in isLoaded: %v\n", err).Log()
	}
	return loaded
}

func (nr *NamespaceRegistry) getSpecsDirForIO() string {
	var specsDir string
	if err := concurrency.RunInLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryGetSpecsDir,
		lockLoggerSystem(),
		func() error {
			specsDir = nr.specsDir
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// loadNamespaceMappings loads namespace mappings from specs directory
			Error("lock failed in getSpecsDirForIO: %v\n", err).Log()
	}
	return specsDir
}

func (nr *NamespaceRegistry) loadNamespaceMappings(specsDir string) map[string]string {
	newMappings := nr.getDefaultNamespaces()
	for k, v := range loadSpecNamespaceOverlays(specsDir) {
		newMappings[k] = v
	}
	return newMappings
}

func loadSpecNamespaceOverlays(specsDir string) map[string]string {
	if specsDir == emptyValue {
		return map[string]string{}
	}
	files := listNamespaceSpecFiles(specsDir)
	stamp := stampmemo.OfAll(append([]string{specsDir}, files...)...)
	overlays, _ := specNamespaceOverlays.Load(specsDir, stamp, func() (map[string]string, error) {
		return readSpecNamespaceOverlays(files), nil
	})
	return cloneStringMap(overlays)
}

func listNamespaceSpecFiles(specsDir string) []string {
	entries, err := fileutil.ReadDir(specsDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		files = append(files, filepath.Join(specsDir, name))
	}
	return files
}

func readSpecNamespaceOverlays(files []string) map[string]string {
	out := make(map[string]string, len(files))
	for _, specPath := range files {
		baseName := strings.TrimSuffix(filepath.Base(specPath), filepath.Ext(specPath))
		if shouldSkipNamespaceSpec(baseName) {
			continue
		}
		namespace := namespaceFromSpecFile(specPath, baseName)
		if namespace != emptyValue {
			out[baseName] = namespace
		}
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func shouldSkipNamespaceSpec(baseName string) bool {
	config := objects.GetGlobalKindMappingsConfig()
	return config.ShouldSkipSpec(baseName)
}

// loadSubordinateNamespaces loads subordinate namespace relationships from config
func (nr *NamespaceRegistry) loadSubordinateNamespaces() map[string][]string {
	config := GetGlobalNamespacesConfig()
	subordinateMap := make(map[string][]string)
	if config == nil {
		return subordinateMap
	}

	for namespaceID, nsConfig := range config.Namespaces {
		if len(nsConfig.SubordinateNamespaces) > 0 {
			subordinateMap[namespaceID] = nsConfig.SubordinateNamespaces
		}
	}

	return subordinateMap
}

// updateRegistry updates the registry with new mappings and subordinate namespaces
func (nr *NamespaceRegistry) updateRegistry(newMappings map[string]string, subordinateMap map[string][]string) error {
	return concurrency.RunInLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryUpdate,
		lockLoggerSystem(),
		func() error {
			// Double-check in case another goroutine loaded while we were doing I/O
			if nr.loaded {
				return nil
			}

			// Update mappings
			for k, v := range newMappings {
				nr.kindToNamespace[k] = v
			}

			// Update subordinate namespace relationships
			for parent, subordinates := range subordinateMap {
				nr.subordinateNamespaces[parent] = subordinates
			}

			nr.loaded = true
			return nil
		},
	)
}

// loadNamespaceFromSpec attempts to extract namespace information from a spec file
func (nr *NamespaceRegistry) loadNamespaceFromSpec(specPath, kind string) string {
	return namespaceFromSpecFile(specPath, kind)
}

func namespaceFromSpecFile(specPath, kind string) string {
	data, err := fileutil.ReadFile(specPath)
	if err != nil {
		return ""
	}

	var spec struct {
		Ontology    string `yaml:"ontology"`
		Kind        string `yaml:"kind"`
		Namespace   string `yaml:"namespace"`
		NamespaceID string `yaml:"namespace_id"`
		Fields      struct {
			NamespaceID struct {
				Validation struct {
					Default string `yaml:"default"`
				} `yaml:"validation"`
			} `yaml:"namespace_id"`
		} `yaml:"fields"`
	}

	if err := yaml.Unmarshal(data, &spec); err != nil {
		return ""
	}

	// Use ontology if present, otherwise fall back to kind
	objectKind := spec.Ontology
	if objectKind == emptyValue {
		objectKind = spec.Kind
	}
	if objectKind == emptyValue {
		objectKind = kind
	}

	// Check for explicit namespace_id in spec
	if spec.NamespaceID != emptyValue {
		return spec.NamespaceID
	}

	// Check for namespace field
	if spec.Namespace != emptyValue {
		return spec.Namespace
	}

	// Check for default namespace_id in fields
	if spec.Fields.NamespaceID.Validation.Default != emptyValue {
		return spec.Fields.NamespaceID.Validation.Default
	}

	// Infer namespace from ontology/kind
	return inferNamespaceFromKindConfig(objectKind)
}

// inferNamespaceFromKind infers the default namespace for a given object kind
// Uses namespaces configuration instead of hardcoded patterns
// Checks registered namespaces first, then falls back to config inference
func (nr *NamespaceRegistry) inferNamespaceFromKind(kind string) string {
	// Check registered namespaces first (runtime registrations take precedence)
	var namespace string
	var found bool
	if err := concurrency.RunInRLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryInferFromKind,
		lockLoggerSystem(),
		func() error {
			var ok bool
			namespace, ok = nr.kindToNamespace[kind]
			found = ok
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("lock failed in inferNamespaceFromKind: %v\n", err).Log()
	}

	if found {
		return namespace
	}

	return inferNamespaceFromKindConfig(kind)
}

func inferNamespaceFromKindConfig(kind string) string {
	config := GetGlobalNamespacesConfig()
	if config != nil {
		return config.GetNamespaceForKind(kind)
	}
	return DefaultNamespaceKernel
}

// getDefaultNamespaces returns default namespace mappings for known object kinds
// Uses namespaces configuration instead of hardcoded lists
func (nr *NamespaceRegistry) getDefaultNamespaces() map[string]string {
	mappings := make(map[string]string)

	// Load from namespaces config
	config := GetGlobalNamespacesConfig()
	if config == nil {
		config = getDefaultNamespacesConfig()
	}

	// Build mappings from config
	for namespaceID, nsConfig := range config.Namespaces {
		for _, kind := range nsConfig.Kinds {
			mappings[kind] = namespaceID
		}
	}

	return mappings
}

// loadDefaultNamespaces loads default namespace mappings for known object kinds
// This is a legacy method that modifies the struct directly (requires lock)
//
//nolint:unused // Reserved for future use or legacy compatibility
func (nr *NamespaceRegistry) loadDefaultNamespaces() {
	mappings := nr.getDefaultNamespaces()
	for k, v := range mappings {
		nr.kindToNamespace[k] = v
	}
}

// GetNamespaceForKind returns the default namespace for an object kind
// Returns DefaultNamespaceKernel if kind is unknown (backward compatibility)
func (nr *NamespaceRegistry) GetNamespaceForKind(kind string) string {
	var loaded bool
	if err := concurrency.RunInRLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryGetForKindCheck,
		lockLoggerSystem(),
		func() error {
			loaded = nr.loaded
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("lock failed in GetNamespaceForKind: %v\n", err).Log()
	}

	if !loaded {
		if err := nr.LoadNamespaces(); err != nil {
			// If loading fails, use config default
			config := GetGlobalNamespacesConfig()
			if config != nil {
				return config.DefaultNamespace
			}
			return DefaultNamespaceKernel // Ultimate fallback
		}
	}

	var namespace string
	var found bool
	if err := concurrency.RunInRLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryGetForKind,
		lockLoggerSystem(),
		func() error {
			var ok bool
			namespace, ok = nr.kindToNamespace[kind]
			found = ok
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("lock failed in GetNamespaceForKind: %v\n", err).Log()
	}

	if found {
		return namespace
	}

	// Unknown kind - infer namespace
	return nr.inferNamespaceFromKind(kind)
}

// GetSubordinateNamespaces returns all subordinate namespaces for a parent namespace
func (nr *NamespaceRegistry) GetSubordinateNamespaces(parentNamespaceID string) []string {
	var loaded bool
	if err := concurrency.RunInRLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryGetSubordinateCheck,
		lockLoggerSystem(),
		func() error {
			loaded = nr.loaded
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Load namespaces if not already loaded
			Error("lock failed in GetSubordinateNamespaces: %v\n", err).Log()
	}

	if !loaded {

		if err := nr.LoadNamespaces(); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to LoadNamespaces in GetSubordinateNamespaces: %v\n", err).Log()
		}
	}

	var subordinates []string
	var found bool
	if err := concurrency.RunInRLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryGetSubordinate,
		lockLoggerSystem(),
		func() error {
			var ok bool
			subordinates, ok = nr.subordinateNamespaces[parentNamespaceID]
			found = ok
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("lock failed in GetSubordinateNamespaces: %v\n", err).Log()
	}

	if found {
		return subordinates
	}
	return []string{}
}

// IsSubordinateOf checks if a namespace is subordinate to another
func (nr *NamespaceRegistry) IsSubordinateOf(childNamespaceID, parentNamespaceID string) bool {
	return strings.HasPrefix(childNamespaceID, parentNamespaceID+":")
}

// GetParentNamespace extracts the parent namespace from a subordinate namespace ID
func (nr *NamespaceRegistry) GetParentNamespace(subordinateNamespaceID string) string {
	parts := strings.Split(subordinateNamespaceID, ":")
	if len(parts) < 2 {
		return "" // Invalid format
	}
	// Return everything except the last part
	return strings.Join(parts[:len(parts)-1], ":")
}

// RegisterNamespace registers a namespace for an object kind
// This allows runtime registration of namespaces
func (nr *NamespaceRegistry) RegisterNamespace(kind, namespaceID string) {
	if err := concurrency.RunInLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryRegister,
		lockLoggerSystem(),
		func() error {
			nr.kindToNamespace[kind] = namespaceID
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// GetAllNamespaces returns all registered namespace mappings
			ProfileSystem))).Error("lock failed in RegisterNamespace: %v\n", err).Log()
	}
}

func (nr *NamespaceRegistry) GetAllNamespaces() map[string]string {
	var result map[string]string
	if err := concurrency.RunInRLockWithLogger(
		&nr.mu,
		LockNameNamespaceRegistryGetAll,
		lockLoggerSystem(),
		func() error {
			result = make(map[string]string)
			for k, v := range nr.kindToNamespace {
				result[k] = v
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("lock failed in GetAllNamespaces: %v\n", err).Log()
	}
	return result
}
