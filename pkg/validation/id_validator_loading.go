package validation

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/loader"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

type specIDFile struct {
	Ontology   string
	Kind       string
	IDTemplate string
	IDPrefixes []string
	IDPattern  string
	skip       bool
}

var specIDFiles stampmemo.Table[specIDFile] // keyed by spec path (closed kind-file set)

// getPatternsRunner returns the shared loader.Runner for ID patterns (lazily created).
func (v *IDValidator) getPatternsRunner() *loader.Runner {
	v.patternsRunnerOnce.Do(func() {
		v.patternsRunner = loader.NewRunner("id_patterns", func(ctx context.Context) error {
			return v.doLoadPatterns(ctx)
		})
	})
	return v.patternsRunner
}

// LoadPatterns loads ID patterns from object specs via the shared loader pattern
// (atomics + completion channel + configurable timeout). See pkg/loader and COMPONENT_LOADER_PATTERN.md.
func (v *IDValidator) LoadPatterns() error {
	return v.getPatternsRunner().Load(pkgctx.NewSystemContext())
}

// doLoadPatterns performs the actual I/O: graph, then files, then defaults. Called by loader.Runner.
func (v *IDValidator) doLoadPatterns(ctx context.Context) error {
	var specsDir string
	var useGraph bool
	var graphConn provider.GraphConnection
	if err := concurrency.RunInRLockWithLogger(
		&v.mu,
		LockNameIdValidatorLoadPatternsGetConfig,
		lockLoggerSystem(),
		func() error {
			specsDir = v.specsDir
			useGraph = v.useGraph
			graphConn = v.graphConn
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Error acquiring read lock for ID patterns config: %v\n", err).Log()
	}

	loadCtx, loadCancel := context.WithTimeout(ctx, 15*time.Second)
	defer loadCancel()

	var newPatterns map[string]*IDPatternConfig
	var loadErr error

	if useGraph && graphConn != nil {
		graphCtx, graphCancel := context.WithTimeout(loadCtx, 2*time.Second)
		defer graphCancel()
		newPatterns, loadErr = v.loadPatternsFromGraphUnlocked(graphCtx, graphConn)
		if loadErr == nil {
			if err := concurrency.RunInLockWithLogger(
				&v.mu,
				LockNameIdValidatorLoadPatternsSetGraph,
				lockLoggerSystem(),
				func() error {
					v.patterns = newPatterns
					v.ensureDefaultPatterns()
					return nil
				},
			); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Error acquiring write lock for ID patterns (graph): %v\n", err).Log()
			}
			return nil
		}
	}

	if specsDir != emptyValue {
		newPatterns, loadErr = v.loadPatternsFromSpecsUnlocked(specsDir)
		if loadErr == nil {
			if err := concurrency.RunInLockWithLogger(
				&v.mu,
				LockNameIdValidatorLoadPatternsSetSpecs,
				lockLoggerSystem(),
				func() error {
					v.patterns = newPatterns
					v.ensureDefaultPatterns()
					return nil
				},
			); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Error acquiring write lock for ID patterns (specs): %v\n", err).Log()
			}
			return nil
		}
	}

	if err := concurrency.RunInLockWithLogger(
		&v.mu,
		LockNameIdValidatorLoadPatternsSetDefaults,
		lockLoggerSystem(),
		func() error {
			v.loadDefaultPatterns()
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Error acquiring write lock for ID patterns (defaults): %v\n", err).Log()
	}

	if loadErr != nil {
		return errfmt.Newf("failed to load ID patterns").Wrap(loadErr)
	}
	return nil
}

// ReloadPatterns forces a reload of ID patterns by resetting the loader state and running Load again.
func (v *IDValidator) ReloadPatterns() error {
	ResetGlobalIDPrefixesConfig()
	v.getPatternsRunner().ResetLoaded()
	return v.getPatternsRunner().Load(pkgctx.NewSystemContext())
}

// loadPatternsFromSpecs loads patterns from YAML spec files
// Must be called with lock held
//
//nolint:unused // Called conditionally in LoadPatterns - may not be detected by static analysis
func (v *IDValidator) loadPatternsFromSpecs() error {
	patterns, err := v.loadPatternsFromSpecsUnlocked(v.specsDir)
	if err != nil {
		return err
	}
	// Copy patterns to v.patterns (lock is held by caller)
	for k, val := range patterns {
		v.patterns[k] = val
	}
	return nil
}

// loadPatternsFromSpecsUnlocked loads patterns from YAML spec files without requiring lock
func (v *IDValidator) loadPatternsFromSpecsUnlocked(specsDir string) (map[string]*IDPatternConfig, error) {
	patterns := make(map[string]*IDPatternConfig)

	if _, err := fileutil.Stat(specsDir); err != nil {
		return patterns, err
	}

	eventLogger := v.getLogger()
	logging.FluentEvent(eventLogger).Debug("Loading ID patterns from specs").
		String("specs_dir", specsDir).
		Log()

	err := filepath.Walk(specsDir, func(path string, info fileutil.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil {
			return nil
		}
		if info.IsDir() || (!strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml")) {
			return nil
		}
		// Skip macOS AppleDouble/resource-fork files (._*); they are not valid YAML and cause "control characters are not allowed"
		if appledouble.SkipNameInReadDir(info.Name()) {
			return nil
		}

		// Skip abstract base specs (auditable, base_object, work_interval, work_unit). Concrete kinds use kind_mappings skip_specs.
		kindMappingsConfig := objects.GetGlobalKindMappingsConfig()
		if kindMappingsConfig != nil && kindMappingsConfig.ShouldSkipSpec(info.Name()) {
			return nil
		}

		config, err := v.parseSpecFile(path)
		if err != nil {
			// Skip files that can't be parsed
			logging.FluentEvent(eventLogger).Warn("Failed to parse spec file").
				File(info.Name()).
				WithError(err).
				Log()
			return nil
		}
		if config != nil {
			patterns[config.Kind] = config
			logging.FluentEvent(eventLogger).Debug("Loaded ID pattern").
				Kind(config.Kind).
				String("prefixes", fmt.Sprintf("%v", config.Prefixes)).
				Log()
		}
		return nil
	})
	if err != nil {
		return patterns, err
	}

	return patterns, nil
}

// getLogger returns a cached event logger, initializing it if needed
// This avoids creating new loggers (and calling AddDestination) repeatedly
func (v *IDValidator) getLogger() *logging.EventLogger {
	var logger *logging.EventLogger
	if err := concurrency.RunInRLockWithLogger(
		&v.loggerMu,
		LockNameIdValidatorGetLoggerCheck,
		lockLoggerSystem(),
		func() error {
			logger = v.eventLogger
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Error acquiring read lock for ID validator logger: %v\n", err).Log()
	}

	if logger != nil {
		return logger
	}

	// Initialize logger (double-check pattern)
	if err := concurrency.RunInLockWithLogger(
		&v.loggerMu,
		LockNameIdValidatorGetLoggerInit,
		lockLoggerSystem(),
		func() error {
			if v.eventLogger == nil {
				v.eventLogger = logging.NewEventLogger(pkgctx.NewSystemContext())
			}
			logger = v.eventLogger
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// parseSpecFile parses a spec file and extracts ID pattern configuration
			Error("Error acquiring write lock for ID validator logger initialization: %v\n", err).Log()
	}
	return logger
}

func readSpecIDFile(path string) (specIDFile, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return specIDFile{}, err
	}

	var spec struct {
		Ontology   string   `yaml:"ontology"`
		Kind       string   `yaml:"kind"`
		IDTemplate string   `yaml:"id_template"`
		IDPrefixes []string `yaml:"id_prefixes"`
		Fields     struct {
			ID struct {
				Validation struct {
					Pattern string `yaml:"pattern"`
				} `yaml:"validation"`
			} `yaml:"id"`
		} `yaml:"fields"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return specIDFile{}, err
	}

	ontology := spec.Ontology
	if ontology == emptyValue {
		ontology = spec.Kind
	}
	header := specIDFile{
		Ontology:   spec.Ontology,
		Kind:       spec.Kind,
		IDTemplate: spec.IDTemplate,
		IDPrefixes: append([]string(nil), spec.IDPrefixes...),
		IDPattern:  spec.Fields.ID.Validation.Pattern,
		skip:       ontology == emptyValue,
	}
	return header, nil
}

func (v *IDValidator) parseSpecFile(path string) (*IDPatternConfig, error) {
	eventLogger := v.getLogger()

	spec, err := specIDFiles.Load(path, stampmemo.Of(path), func() (specIDFile, error) {
		return readSpecIDFile(path)
	})
	if err != nil {
		logging.FluentEvent(eventLogger).Warn("Failed to read spec file").
			File(path).
			WithError(err).
			Log()
		return nil, err
	}
	spec.IDPrefixes = append([]string(nil), spec.IDPrefixes...)

	if spec.skip {
		logging.FluentEvent(eventLogger).Debug("Spec file has no ontology or kind, skipping").
			File(filepath.Base(path)).
			Log()
		return nil, nil
	}

	// Use ontology if present, otherwise fall back to kind
	ontology := spec.Ontology
	if ontology == emptyValue {
		ontology = spec.Kind
	}

	if ontology == emptyValue {
		logging.FluentEvent(eventLogger).Debug("Spec file has no ontology or kind, skipping").
			File(filepath.Base(path)).
			Log()
		return nil, nil
	}

	config := &IDPatternConfig{
		Kind: ontology,
	}

	// Use id_prefixes if explicitly provided (takes precedence)
	if len(spec.IDPrefixes) > 0 {
		config.Prefixes = spec.IDPrefixes
		logging.FluentEvent(eventLogger).Debug("Using id_prefixes from spec").
			String("kind", ontology).
			String("prefixes", fmt.Sprintf("%v", config.Prefixes)).
			Log()
	}

	// Extract prefixes from id_template if not already set (e.g., "BLI-{sequence}" -> ["BLI-"])
	if len(config.Prefixes) == 0 && spec.IDTemplate != emptyValue {
		if idx := strings.Index(spec.IDTemplate, "-"); idx > 0 {
			prefix := spec.IDTemplate[:idx+1]
			config.Prefixes = []string{prefix}
			config.Template = spec.IDTemplate
			logging.FluentEvent(eventLogger).Debug("Extracted prefix from id_template").
				String("kind", ontology).
				String("id_template", spec.IDTemplate).
				String("prefix", prefix).
				Log()
		}
	} else if spec.IDTemplate != emptyValue {
		// Still set template even if prefixes came from id_prefixes
		config.Template = spec.IDTemplate
	}

	// Extract pattern from id field validation
	if spec.IDPattern != emptyValue {
		config.Pattern = spec.IDPattern
		// Extract prefixes from pattern if not already set
		if len(config.Prefixes) == 0 {
			config.Prefixes = extractPrefixesFromPattern(spec.IDPattern)
		}
	}

	// If no prefixes found, try to infer from ontology name
	// Note: inferPrefixesFromKind checks config first, so config values will be used if available
	if len(config.Prefixes) == 0 {
		config.Prefixes = v.inferPrefixesFromKind(spec.Ontology)
		logging.FluentEvent(eventLogger).Debug("Inferred prefix from ontology name").
			String("kind", ontology).
			String("prefixes", fmt.Sprintf("%v", config.Prefixes)).
			Log()
	}

	// CRITICAL: Always ensure config values take precedence (even if we just inferred)
	// This handles cases where config is loaded after spec parsing
	// Config is the source of truth for prefixes - it can define multiple valid prefixes
	// NOTE: We check config here, but ensureDefaultPatterns() will also override after all specs are loaded
	// This ensures we get the latest config even if it was loaded after this spec was parsed
	// IMPORTANT: Only override with instance/global config if:
	// 1. The spec file doesn't have explicit id_prefixes, OR
	// 2. The config has an explicit mapping (not inferred)
	// This ensures spec file id_prefixes take precedence over inferred config prefixes
	// Use instance config if available (for test isolation), otherwise fall back to global
	globalConfig := v.getIDPrefixesConfig()
	if globalConfig != nil {
		// Only override if global config has an explicit mapping (not inferred)
		// OR if spec file doesn't have id_prefixes
		hasExplicitConfigMapping := globalConfig.HasExplicitMapping(spec.Ontology)
		specHasPrefixes := len(spec.IDPrefixes) > 0

		if hasExplicitConfigMapping || !specHasPrefixes {
			if configPrefixes := globalConfig.GetPrefixesForKind(spec.Ontology); len(configPrefixes) > 0 {
				// Config is authoritative - use config prefixes
				// This allows config to define alternative prefixes (e.g., ADR- and DEC- for decision)
				// But only override spec prefixes if config has explicit mapping
				// CRITICAL: Preserve pattern from spec - it can validate IDs that match pattern but not in prefix list
				// (e.g., POL-AGENT-001 matches ^POL-[A-Z]+- pattern but POL-AGENT- not in prefix list)
				originalPattern := config.Pattern // Preserve pattern before overriding prefixes
				config.Prefixes = configPrefixes
				config.Pattern = originalPattern // Restore pattern after setting prefixes
				// Keep pattern for validation, but prefixes take precedence for matching
				// Pattern will be used as fallback if prefixes don't match (see validateActualID)
				logging.FluentEvent(eventLogger).Debug("Overriding with config prefixes (pattern preserved for fallback)").
					String("kind", ontology).
					String("prefixes", fmt.Sprintf("%v", config.Prefixes)).
					String("pattern", config.Pattern).
					String("explicit_config_mapping", fmt.Sprintf("%v", hasExplicitConfigMapping)).
					String("spec_had_prefixes", fmt.Sprintf("%v", specHasPrefixes)).
					Log()
			}
		} else {
			// Spec file has explicit id_prefixes and config doesn't have explicit mapping
			// Keep spec prefixes (already set above)
			logging.FluentEvent(eventLogger).Debug("Keeping spec file id_prefixes (config has no explicit mapping)").
				String("kind", ontology).
				String("spec_prefixes", fmt.Sprintf("%v", config.Prefixes)).
				Log()
		}
	}

	if len(config.Prefixes) > 0 {
		logging.FluentEvent(eventLogger).Debug("Successfully parsed ID pattern").
			Kind(config.Kind).
			String("prefixes", fmt.Sprintf("%v", config.Prefixes)).
			String("id_template", config.Template).
			Log()
	}

	return config, nil
}

// loadDefaultPatterns loads default patterns from config
// All patterns are now externalized to id_prefixes_config.yaml
// Uses instance config if available (for test isolation), otherwise falls back to global
func (v *IDValidator) loadDefaultPatterns() {
	config := v.getIDPrefixesConfig()
	if config == nil {
		return
	}

	// Load all patterns from config
	for kind, prefixes := range config.KindToPrefixes {
		if len(prefixes) > 0 {
			v.patterns[kind] = &IDPatternConfig{
				Kind:     kind,
				Prefixes: prefixes,
			}
		}
	}
}

// ensureDefaultPatterns ensures default patterns exist even if not in specs
// Config-based prefixes take precedence when config has values, otherwise keep spec-based prefixes
// Uses instance config if available (for test isolation), otherwise falls back to global
func (v *IDValidator) ensureDefaultPatterns() {
	config := v.getIDPrefixesConfig()
	if config == nil {
		return
	}

	// Config-based prefixes take precedence when config has values
	// If config is empty, keep existing spec-based prefixes
	for kind, configPrefixes := range config.KindToPrefixes {
		if len(configPrefixes) == 0 {
			continue
		}

		if _, exists := v.patterns[kind]; !exists {
			// Pattern doesn't exist, create it from config
			v.patterns[kind] = &IDPatternConfig{
				Kind:     kind,
				Prefixes: configPrefixes,
			}
		} else if len(configPrefixes) > 0 {
			// Pattern exists - use config prefixes if config has values (config is source of truth)
			// This ensures externalized config takes precedence over spec file prefixes
			// But only when config actually has values (don't overwrite with empty)
			// Always overwrite with config values when config has them (config is authoritative)
			v.patterns[kind].Prefixes = configPrefixes
			// Keep pattern for fallback validation (e.g., POL-[A-Z]+- allows any category)
			// Pattern will be used if prefixes don't match (see validateActualID)
			// Don't clear pattern - it can validate IDs that match pattern but not in prefix list
		}
	}
}

// loadPatternsFromGraph loads ID patterns from graph backend
// Queries for ObjectSpec nodes with label "ObjectSpec" or "Spec"
// Must be called with lock held
//
//nolint:unused // Called conditionally in LoadPatterns - may not be detected by static analysis
func (v *IDValidator) loadPatternsFromGraph(ctx context.Context) error {
	if v.graphConn == nil {
		return errfmt.Errorf("graph connection not available")
	}
	patterns, err := v.loadPatternsFromGraphUnlocked(ctx, v.graphConn)
	if err != nil {
		return err
	}
	// Copy patterns to v.patterns (lock is held by caller)
	for k, val := range patterns {
		v.patterns[k] = val
	}
	return nil
}

// loadPatternsFromGraphUnlocked loads ID patterns from graph backend without requiring lock
func (v *IDValidator) loadPatternsFromGraphUnlocked(ctx context.Context, graphConn provider.GraphConnection) (map[string]*IDPatternConfig, error) {
	patterns := make(map[string]*IDPatternConfig)

	if graphConn == nil {
		return patterns, errfmt.Errorf("graph connection not available")
	}

	// Query for all ObjectSpec nodes
	filter := provider.NodeFilter{
		Labels: []string{"ObjectSpec", "Spec"}, // Try both label names
		Limit:  1000,                           // Reasonable limit for specs
	}

	nodes, err := graphConn.ListNodes(ctx, filter)
	if err != nil {
		return patterns, errfmt.Newf("failed to query graph for specs").Wrap(err)
	}

	// If no nodes found with those labels, try querying by ontology property
	if len(nodes) == 0 {
		filter = provider.NodeFilter{
			Properties: map[string]any{
				objects.FieldKeyType: objects.KindObjectSpec, // Alternative: look for type property
			},
			Limit: 1000,
		}
		nodes, err = graphConn.ListNodes(ctx, filter)
		if err != nil {
			return patterns, errfmt.Newf("failed to query graph for specs").Wrap(err)
		}
	}

	// Parse each spec node
	for _, node := range nodes {
		config := v.parseGraphSpecNode(node)
		if config != nil {
			patterns[config.Kind] = config
		}
	}

	return patterns, nil
}

// parseGraphSpecNode parses a graph node into an IDPatternConfig
func (v *IDValidator) parseGraphSpecNode(node *provider.Node) *IDPatternConfig {
	// Extract ontology/kind from node properties
	ontology, ok := node.Properties[objects.FieldKeyOntology].(string)
	if !ok {
		var shouldReturn bool
		when.When(func() bool {
			kind := objects.GetString(node.Properties, objects.FieldKeyKind)
			ontology = kind
			return kind != ""
		}).Then(func() {
			// Done
		}).OrElseWhen(func() bool {
			kind := objects.GetString(node.Properties, objects.FieldKeyType)
			ontology = kind
			return kind != ""
		}).Then(func() {
			// Done
		}).OrElse(func() {
			shouldReturn = true
		}).Run()

		if shouldReturn {
			return nil
		}
	}

	if ontology == emptyValue {
		return nil
	}

	config := &IDPatternConfig{
		Kind: ontology,
	}

	// Extract id_template
	if template := objects.GetString(node.Properties, "id_template"); template != emptyValue {
		config.Template = template
		// Extract prefix from template (e.g., "BLI-{sequence}" -> "BLI-")
		if idx := strings.Index(template, "-"); idx > 0 {
			prefix := template[:idx+1]
			config.Prefixes = []string{prefix}
		}
	}

	// Extract validation pattern
	// Handle nested structure: fields.id.validation.pattern
	if fields, ok := node.Properties["fields"].(map[string]any); ok {
		if idField, ok := fields[objects.FieldKeyID].(map[string]any); ok {
			if validation, ok := idField["validation"].(map[string]any); ok {
				if pattern := objects.GetString(validation, "pattern"); pattern != emptyValue {
					config.Pattern = pattern
					// Extract prefixes from pattern if not already set
					if len(config.Prefixes) == 0 {
						config.Prefixes = extractPrefixesFromPattern(pattern)
					}
				}
			}
		}
	}

	// Extract prefixes directly (if stored as array)
	if prefixes, ok := node.Properties["id_prefixes"].([]any); ok && len(prefixes) > 0 {
		config.Prefixes = make([]string, 0, len(prefixes))
		for _, p := range prefixes {
			if prefix, ok := p.(string); ok {
				config.Prefixes = append(config.Prefixes, prefix)
			}
		}
	}

	// If no prefixes found, try to infer from ontology
	// Note: parseGraphSpecNode doesn't have validator instance, so we use global config fallback
	// This is acceptable since graph loading is typically not used in parallel test scenarios
	if len(config.Prefixes) == 0 {
		// Use global config for inference (graph loading scenarios don't typically need test isolation)
		globalConfig := GetGlobalIDPrefixesConfig()
		if globalConfig != nil {
			if prefixes := globalConfig.GetPrefixesForKind(ontology); len(prefixes) > 0 {
				config.Prefixes = prefixes
			}
		}
		// Fallback: try to infer from kind name (backward compatibility)
		if len(config.Prefixes) == 0 {
			parts := strings.Split(ontology, "_")
			if len(parts) > 0 {
				firstPart := strings.ToUpper(parts[0])
				if len(firstPart) >= 3 {
					prefix := firstPart[:3] + "-"
					config.Prefixes = []string{prefix}
				}
			}
		}
	}

	return config
}
