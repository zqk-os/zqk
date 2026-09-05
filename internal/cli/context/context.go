package context

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli/flagutil"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
)

// ContextLayer represents the precedence layer for configuration
type ContextLayer int

const (
	// LayerSystem represents system defaults (lowest precedence)
	LayerSystem ContextLayer = iota
	// LayerUser represents user-level configuration (~/.zqk/config/config.yaml)
	LayerUser
	// LayerProject represents project-level configuration (.zqk/config/config.yaml)
	LayerProject
	// LayerCommand represents command-line flags (highest precedence)
	LayerCommand
)

const (
	contextKeyFormat            = "format"
	contextKeyVerbose           = "verbose"
	contextKeyQuiet             = "quiet"
	contextKeyProfile           = "profile"
	contextKeyPriorityPlan      = objects.KindPriorityPlan
	contextKeyWorkstream        = objects.KindWorkstream
	contextKeyMilestone         = objects.KindMilestone
	contextKeyStorage           = "storage"
	contextKeyFlags             = "flags"
	contextKeyCommands          = "commands"
	contextKeyMaxPageSize       = "max_page_size"
	contextKeyDefaultPageSize   = "default_page_size"
	contextKeyEnableGrouping    = "enable_grouping"
	contextKeyMaxGroupSize      = "max_group_size"
	contextKeyProjectRoot       = "project_root"
	contextKeyProductName       = "product_name"
	contextKeyExecutableName    = "executable_name"
	contextKeyNamespacePrefix   = "namespace_prefix"
	contextKeyBrand             = objects.KindBrand
	contextKeyCacheFreshEnabled = "cache_freshness_enabled"
	contextKeyCacheFreshTrig    = "cache_freshness_triggers"
	contextKeyCacheFreshMinInt  = "cache_freshness_min_interval"
	contextKeyErrorLogOutput    = "error_log_output"
)

// Context represents the CLI execution context
type Context struct {
	// Output preferences
	Format  string // Output format (table, json, yaml)
	Verbose bool
	Quiet   bool

	// Context profiles (e.g., "ai-agent" sets format to JSON)
	Profile string

	// Project context
	ProjectRoot  string
	PriorityPlan string
	Workstream   string
	Milestone    string

	// Branding (white-label)
	// ProductName is the human-facing product name (e.g., "ZQK").
	ProductName string
	// ExecutableName is the CLI executable name used in command examples (e.g., "zqk", "acme").
	ExecutableName string
	// NamespacePrefix is the prefix used for kernel namespaces (e.g., "zqk", "acme").
	// This is separate from product name and executable name.
	NamespacePrefix string

	// Storage context parameters (for pagination, grouping, etc.)
	StorageMaxPageSize     int  // Maximum items per page (0 = no limit)
	StorageDefaultPageSize int  // Default page size when not specified (0 = no default)
	StorageEnableGrouping  bool // Whether grouping is enabled
	StorageMaxGroupSize    int  // Maximum groups to return (0 = no limit)

	// Cache freshness policy
	CacheFreshnessEnabled     bool     // Whether cache freshness checks are enabled (default: true)
	CacheFreshnessTriggers    []string // Operations that trigger freshness checks (create, update, delete, bulk_*)
	CacheFreshnessMinInterval int      // Minimum seconds between freshness checks (0 = no limit, default: 5)

	// Log capture: when writing command output to log files, use "combined" (stdout+stderr in one stream)
	// or "separate" (stdout and stderr to separate files). From logging.error_log_output in project config.
	ErrorLogOutput string // "combined" | "separate", default "combined"

	// pathResolver when non-nil overrides [Context.PathResolver] (tests inject mocks; production leaves nil).
	pathResolver paths.PathResolver

	// Configuration layers (for debugging/inspection)
	layers map[ContextLayer]map[string]any
}

// GetLayers returns the layers map for inspection
// This allows external code to see which layer provided which values
func (c *Context) GetLayers() map[ContextLayer]map[string]any {
	if c.layers == nil {
		return make(map[ContextLayer]map[string]any)
	}
	return c.layers
}

// Derive creates a new context derived from this context with optional overrides
// This allows functions to work with a single starting context and derive variations
// without needing multiple context objects passed in
func (c *Context) Derive(overrides map[string]any) *Context {
	// Clone the current context
	processor := NewContextProcessor(ModeSequential)
	derived := processor.cloneContext(c)

	// Apply overrides if provided
	if len(overrides) > 0 {
		overrideCtx := &Context{
			layers: make(map[ContextLayer]map[string]any),
		}
		overrideCtx.layers[LayerCommand] = overrides

		// Apply format
		if format, ok := overrides[contextKeyFormat].(string); ok && format != emptyValue {
			overrideCtx.Format = format
		}
		// Apply verbose
		if verbose, ok := overrides[contextKeyVerbose].(bool); ok {
			overrideCtx.Verbose = verbose
		}
		// Apply quiet
		if quiet, ok := overrides[contextKeyQuiet].(bool); ok {
			overrideCtx.Quiet = quiet
		}
		// Apply profile
		if profile, ok := overrides[contextKeyProfile].(string); ok && profile != emptyValue {
			overrideCtx.Profile = profile
		}
		// Apply project context
		if priorityPlan, ok := overrides[contextKeyPriorityPlan].(string); ok && priorityPlan != emptyValue {
			overrideCtx.PriorityPlan = priorityPlan
		}
		if workstream, ok := overrides[contextKeyWorkstream].(string); ok && workstream != emptyValue {
			overrideCtx.Workstream = workstream
		}
		if milestone, ok := overrides[contextKeyMilestone].(string); ok && milestone != emptyValue {
			overrideCtx.Milestone = milestone
		}
		// Apply storage context
		if storageCtx, ok := overrides[contextKeyStorage].(map[string]any); ok {
			switch maxPageSize := storageCtx[contextKeyMaxPageSize].(type) {
			case int:
				overrideCtx.StorageMaxPageSize = maxPageSize
			case float64:
				overrideCtx.StorageMaxPageSize = int(maxPageSize)
			}
			switch defaultPageSize := storageCtx[contextKeyDefaultPageSize].(type) {
			case int:
				overrideCtx.StorageDefaultPageSize = defaultPageSize
			case float64:
				overrideCtx.StorageDefaultPageSize = int(defaultPageSize)
			}
			if enableGrouping, ok := storageCtx[contextKeyEnableGrouping].(bool); ok {
				overrideCtx.StorageEnableGrouping = enableGrouping
			}
			switch maxGroupSize := storageCtx[contextKeyMaxGroupSize].(type) {
			case int:
				overrideCtx.StorageMaxGroupSize = maxGroupSize
			case float64:
				overrideCtx.StorageMaxGroupSize = int(maxGroupSize)
			}
		}

		// Merge the override context into derived
		processor.mergeContext(derived, overrideCtx)
	}

	return derived
}

// WithFormat creates a new context with a different format
func (c *Context) WithFormat(format string) *Context {
	return c.Derive(map[string]any{contextKeyFormat: format})
}

// WithVerbose creates a new context with verbose enabled/disabled
func (c *Context) WithVerbose(verbose bool) *Context {
	return c.Derive(map[string]any{contextKeyVerbose: verbose})
}

// WithQuiet creates a new context with quiet enabled/disabled
func (c *Context) WithQuiet(quiet bool) *Context {
	return c.Derive(map[string]any{contextKeyQuiet: quiet})
}

// WithProfile creates a new context with a different profile
func (c *Context) WithProfile(profile string) *Context {
	return c.Derive(map[string]any{contextKeyProfile: profile})
}

// WithStorageContext creates a new context with different storage settings
func (c *Context) WithStorageContext(maxPageSize, defaultPageSize int, enableGrouping bool, maxGroupSize int) *Context {
	return c.Derive(map[string]any{
		contextKeyStorage: map[string]any{
			contextKeyMaxPageSize:     maxPageSize,
			contextKeyDefaultPageSize: defaultPageSize,
			contextKeyEnableGrouping:  enableGrouping,
			contextKeyMaxGroupSize:    maxGroupSize,
		},
	})
}

// GetErrorLogOutput returns the effective log capture mode: "separate" (stdout and stderr to separate files)
// or "combined" (single stream). Empty config is treated as "combined".
func (c *Context) GetErrorLogOutput() string {
	if c == nil {
		return "combined"
	}
	if c.ErrorLogOutput == "separate" {
		return "separate"
	}
	return "combined"
}

// PathResolver returns a paths.PathResolver for this context's ProjectRoot.
// When pathResolver is set ([WithPathResolver]), that value is returned (for tests).
// Build the path alias cache (scheduler pre-warm, zqk system path-cache, or storage.BuildPathAliasCacheForProject)
// before ResolveStrict when aliases must be resolved.
func (c *Context) PathResolver() paths.PathResolver {
	if c == nil {
		return paths.NewPathResolver("")
	}
	if c.pathResolver != nil {
		return c.pathResolver
	}
	return paths.NewPathResolver(c.ProjectRoot)
}

// WithPathResolver returns a derived context that uses r for [Context.PathResolver] instead of
// [paths.NewPathResolver](ProjectRoot). Pass nil to clear the override.
func (c *Context) WithPathResolver(r paths.PathResolver) *Context {
	if c == nil {
		return nil
	}
	processor := NewContextProcessor(ModeSequential)
	derived := processor.cloneContext(c)
	derived.pathResolver = r
	return derived
}

// ProfileDefinition represents a profile configuration loaded from config files
type ProfileDefinition struct {
	Format      string         `yaml:"format"`
	Verbose     bool           `yaml:"verbose"`
	Quiet       bool           `yaml:"quiet"`
	Flags       map[string]any `yaml:"flags,omitempty"`
	Commands    map[string]any `yaml:"commands,omitempty"`
	Description string         `yaml:"description,omitempty"`
	// Storage context parameters
	StorageMaxPageSize     int  `yaml:"storage_max_page_size,omitempty"`
	StorageDefaultPageSize int  `yaml:"storage_default_page_size,omitempty"`
	StorageEnableGrouping  bool `yaml:"storage_enable_grouping,omitempty"`
	StorageMaxGroupSize    int  `yaml:"storage_max_group_size,omitempty"`
}

// ContextManager manages context loading and precedence
type ContextManager struct {
	systemDefaults map[string]any
	userConfig     map[string]any
	projectConfig  map[string]any
	commandFlags   map[string]any
	// Profile loader for loading versioned profile specs
	profileLoader *ProfileLoader
}

// NewContextManager creates a new context manager
func NewContextManager() *ContextManager {
	return &ContextManager{
		systemDefaults: make(map[string]any),
		userConfig:     make(map[string]any),
		projectConfig:  make(map[string]any),
		commandFlags:   make(map[string]any),
		profileLoader:  NewProfileLoader(""),
	}
}

// LoadContext loads context from all layers and applies precedence using the builder/processor
// initCtx provides CLI initialization context (project root, etc.) - always pass context objects, never extract values
func (cm *ContextManager) LoadContext(initCtx *pkgctx.CliInitializationContext) (*Context, error) {
	// Get project root from context (always valid, no conditional check needed)
	projectRoot := initCtx.GetProjectRoot()

	// Load all context layers
	cm.loadSystemDefaults()

	// Load user config (~/.zqk/config/config.yaml)
	if err := cm.loadUserConfig(); err != nil {
		// User config is optional, continue if missing
	}

	// Load project config (.zqk/config/config.yaml)
	if projectRoot != emptyValue {
		if err := cm.loadProjectConfig(projectRoot); err != nil {
			// Project config is optional, continue if missing
		}
	}

	// Use chain-based context system for DRY precedence handling
	// Build chain of contexts in order of precedence (breadth-first processing)
	chainBuilder := pkgctx.NewChainBuilder()

	// Create context objects from each layer and add to chain
	systemCtx, err := cm.createContextFromMap(cm.systemDefaults, LayerSystem)
	if err != nil {
		return nil, err
	}
	if systemCtx != nil {
		chainable := ToChainableContext(systemCtx, LayerSystem)
		chainBuilder.AddContext(chainable)
	}

	userCtx, err := cm.createContextFromMap(cm.userConfig, LayerUser)
	if err != nil {
		return nil, err
	}
	if userCtx != nil {
		chainable := ToChainableContext(userCtx, LayerUser)
		chainBuilder.AddContext(chainable)
	}

	if projectRoot != emptyValue {
		projectCtx, err := cm.createContextFromMap(cm.projectConfig, LayerProject)
		if err != nil {
			return nil, err
		}
		when.When(func() bool { return projectCtx != nil }).Then(func() {
			projectCtx.ProjectRoot = projectRoot
		}).OrElse(func() {
			projectCtx = &Context{
				layers:      make(map[ContextLayer]map[string]any),
				ProjectRoot: projectRoot,
			}
			projectCtx.layers[LayerProject] = make(map[string]any)
		}).Run()
		chainable := ToChainableContext(projectCtx, LayerProject)
		chainBuilder.AddContext(chainable)
	}

	commandCtx, err := cm.createContextFromMap(cm.commandFlags, LayerCommand)
	if err != nil {
		return nil, err
	}
	if commandCtx != nil {
		chainable := ToChainableContext(commandCtx, LayerCommand)
		chainBuilder.AddContext(chainable)
	}

	// Build the final context using chain-based breadth-first processing
	chainResult, validationErrors := chainBuilder.Build()
	if len(validationErrors) > 0 {
		// Validation errors are non-fatal - context loading continues with best-effort merge
		// Errors are returned in the validationErrors slice for caller inspection if needed
		// In most cases, partial context is better than no context
	}

	// Extract the underlying Context from the adapter
	adapter, ok := chainResult.(*ContextChainAdapter)
	if !ok {
		return nil, errfmt.Errorf("chain builder returned unexpected context type: %T", chainResult)
	}

	result := adapter.GetContext()

	// Ensure project root is set in the final context
	if projectRoot != emptyValue && result.ProjectRoot == emptyValue {
		result.ProjectRoot = projectRoot
	}

	// Apply profile if set (profiles may override format and other settings)
	// This must happen after chain merging to ensure profile settings take precedence
	// Check command flags directly in case Profile wasn't preserved during merge
	var profileValue string
	profileFromFlags, profileFromFlagsOK := cm.commandFlags[contextKeyProfile].(string)
	when.When(func() bool { return result.Profile != emptyValue }).Then(func() {
		profileValue = result.Profile
	}).OrElseWhen(func() bool { return profileFromFlagsOK && profileFromFlags != emptyValue }).Then(func() {
		profileValue = profileFromFlags
		result.Profile = profileValue
	}).Run()
	if profileValue != emptyValue {
		if err := cm.applyProfile(result, profileValue); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// SetCommandFlag sets a command-line flag value (highest precedence)
func (cm *ContextManager) SetCommandFlag(key string, value any) {
	cm.commandFlags[key] = value
}

// loadSystemDefaults loads system default values
func (cm *ContextManager) loadSystemDefaults() {
	cm.systemDefaults = map[string]any{
		contextKeyFormat:          "table",
		contextKeyVerbose:         false,
		contextKeyQuiet:           false,
		contextKeyProfile:         "",
		contextKeyProjectRoot:     "",
		contextKeyPriorityPlan:    "",
		contextKeyWorkstream:      "",
		contextKeyMilestone:       "",
		contextKeyProductName:     "ZQK",
		contextKeyExecutableName:  "",
		contextKeyNamespacePrefix: paths.NamespacePrefixDefault,
		// Cache freshness policy defaults
		contextKeyCacheFreshEnabled: true,
		contextKeyCacheFreshTrig:    []string{"create", "update", "delete", "bulk_create", "bulk_update", "bulk_delete"},
		contextKeyCacheFreshMinInt:  5,          // Minimum 5 seconds between checks
		contextKeyErrorLogOutput:    "combined", // "combined" | "separate" for log capture (logging.error_log_output)
	}
}

// loadUserConfig loads user-level configuration from ~/.zqk/config/config.yaml
// Falls back to legacy ~/.zqk/config/config.yaml for backward compatibility.
func (cm *ContextManager) loadUserConfig() error {
	homeDir, err := fileutil.UserHomeDir()
	if err != nil {
		return err
	}

	primaryPath := filepath.Join(homeDir, ".zqk", "config", "config.yaml")
	cfg, err := cm.loadConfigFile(primaryPath)
	if err != nil {
		return err
	}
	// Backward-compat: if primary config is empty, try legacy path.
	if len(cfg) == 0 {
		legacyPath := filepath.Join(homeDir, ".zqk", "config", "config.yaml")
		if legacyCfg, _ := cm.loadConfigFile(legacyPath); len(legacyCfg) > 0 {
			cfg = legacyCfg
		}
	}
	cm.userConfig = cfg
	return nil
}

// loadProjectConfig loads project-level configuration from .zqk/config/config.yaml
func (cm *ContextManager) loadProjectConfig(projectRoot string) error {
	configPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile)
	cfg, err := cm.loadConfigFile(configPath)
	if err != nil {
		return err
	}
	cm.projectConfig = cfg
	return nil
}

// loadConfigFile loads a YAML configuration file
func (cm *ContextManager) loadConfigFile(path string) (map[string]any, error) {
	data, err := fileutil.ReadFile(path)
	if fileutil.IsNotExist(err) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, err
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errfmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return config, nil
}

// createContextFromMap creates a Context from a map of values
// This is used to convert each layer's config map into a Context object.
// Project root is intentionally not read from config; it comes only from initCtx at CLI init
// (ResolveProjectRoot). See docs/architecture/PROJECT_ROOT_ORIENTATION.md.
func (cm *ContextManager) createContextFromMap(config map[string]any, layer ContextLayer) (*Context, error) {
	if len(config) == 0 {
		return nil, nil
	}

	ctx := &Context{
		layers: make(map[ContextLayer]map[string]any),
	}
	ctx.layers[layer] = config

	// Apply format
	if format, ok := config[contextKeyFormat].(string); ok && format != emptyValue {
		ctx.Format = format
	}

	// Apply verbose
	if verbose, ok := config[contextKeyVerbose].(bool); ok {
		ctx.Verbose = verbose
	}

	// Apply quiet
	if quiet, ok := config[contextKeyQuiet].(bool); ok {
		ctx.Quiet = quiet
	}

	// Apply profile (may override format)
	if profile, ok := config[contextKeyProfile].(string); ok && profile != emptyValue {
		ctx.Profile = profile
		if err := cm.applyProfile(ctx, profile); err != nil {
			return nil, err
		}
	}

	// Apply project context
	if priorityPlan, ok := config[contextKeyPriorityPlan].(string); ok && priorityPlan != emptyValue {
		ctx.PriorityPlan = priorityPlan
	}
	if workstream, ok := config[contextKeyWorkstream].(string); ok && workstream != emptyValue {
		ctx.Workstream = workstream
	}
	if milestone, ok := config[contextKeyMilestone].(string); ok && milestone != emptyValue {
		ctx.Milestone = milestone
	}

	// Apply branding
	if productName, ok := config[contextKeyProductName].(string); ok && productName != emptyValue {
		ctx.ProductName = productName
	}
	if executableName, ok := config[contextKeyExecutableName].(string); ok && executableName != emptyValue {
		ctx.ExecutableName = executableName
	}
	if namespacePrefix, ok := config[contextKeyNamespacePrefix].(string); ok && namespacePrefix != emptyValue {
		ctx.NamespacePrefix = namespacePrefix
	}
	// Allow nested config: brand: { product_name, executable_name }
	if brandCfg, ok := config[contextKeyBrand].(map[string]any); ok {
		if productName, ok := brandCfg[contextKeyProductName].(string); ok && productName != emptyValue {
			ctx.ProductName = productName
		}
		if executableName, ok := brandCfg[contextKeyExecutableName].(string); ok && executableName != emptyValue {
			ctx.ExecutableName = executableName
		}
		if namespacePrefix, ok := brandCfg[contextKeyNamespacePrefix].(string); ok && namespacePrefix != emptyValue {
			ctx.NamespacePrefix = namespacePrefix
		}
	}

	// Apply storage context parameters
	if storageCtx, ok := config[contextKeyStorage].(map[string]any); ok {
		switch maxPageSize := storageCtx[contextKeyMaxPageSize].(type) {
		case int:
			ctx.StorageMaxPageSize = maxPageSize
		case float64:
			// YAML unmarshals integers as float64
			ctx.StorageMaxPageSize = int(maxPageSize)
		}
		switch defaultPageSize := storageCtx[contextKeyDefaultPageSize].(type) {
		case int:
			ctx.StorageDefaultPageSize = defaultPageSize
		case float64:
			// YAML unmarshals integers as float64
			ctx.StorageDefaultPageSize = int(defaultPageSize)
		}
		if enableGrouping, ok := storageCtx[contextKeyEnableGrouping].(bool); ok {
			ctx.StorageEnableGrouping = enableGrouping
		}
		switch maxGroupSize := storageCtx[contextKeyMaxGroupSize].(type) {
		case int:
			ctx.StorageMaxGroupSize = maxGroupSize
		case float64:
			// YAML unmarshals integers as float64
			ctx.StorageMaxGroupSize = int(maxGroupSize)
		}
	}

	// Apply cache freshness policy
	if enabled, ok := config[contextKeyCacheFreshEnabled].(bool); ok {
		ctx.CacheFreshnessEnabled = enabled
	}
	if triggers, ok := config[contextKeyCacheFreshTrig].([]any); ok {
		ctx.CacheFreshnessTriggers = make([]string, 0, len(triggers))
		for _, trigger := range triggers {
			if str, ok := trigger.(string); ok {
				ctx.CacheFreshnessTriggers = append(ctx.CacheFreshnessTriggers, str)
			}
		}
	}
	switch minInterval := config[contextKeyCacheFreshMinInt].(type) {
	case int:
		ctx.CacheFreshnessMinInterval = minInterval
	case float64:
		// YAML unmarshals integers as float64
		ctx.CacheFreshnessMinInterval = int(minInterval)
	}

	if v := resolveErrorLogOutput(config); v != emptyValue {
		ctx.ErrorLogOutput = v
	}

	return ctx, nil
}

// getPositiveInt returns the value for key as an int (handles YAML int or float64). Returns 0 if missing or ≤ 0.
func getPositiveInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

// applyProfile applies a context profile from pkg/cli profile specs (see paths.CLIProfilesDir).
func (cm *ContextManager) applyProfile(ctx *Context, profile string) error {
	ctx.Profile = profile
	return cm.profileLoader.ApplyNamedProfile(ctx, profile)
}

// getFlagFromChain finds a flag by walking up the command chain (Flags then PersistentFlags at each level).
// Must match cli.GetFlagFromChain behavior so root persistent flags are found from nested subcommands.
func getFlagFromChain(cmd *cobra.Command, name string) *pflag.Flag {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.Flags().Lookup(name); f != nil {
			return f
		}
		if f := c.PersistentFlags().Lookup(name); f != nil {
			return f
		}
	}
	return nil
}

func getBoolFlagFromChain(cmd *cobra.Command, name string) bool {
	return flagutil.IsTrue(getFlagFromChain(cmd, name))
}

// GetContextFromCommand extracts context from a cobra command
// initCtx provides CLI initialization context (project root, etc.) - always pass context objects, never extract values
func GetContextFromCommand(cmd *cobra.Command, initCtx *pkgctx.CliInitializationContext) (*Context, error) {
	manager := NewContextManager()

	// Extract command-line flags via chain lookup so root persistent flags (format, context, etc.) are found from nested subcommands
	if formatFlag := getFlagFromChain(cmd, contextKeyFormat); formatFlag != nil && formatFlag.Changed {
		if formatValue := formatFlag.Value.String(); formatValue != emptyValue {
			manager.SetCommandFlag(contextKeyFormat, formatValue)
		}
	}
	if getBoolFlagFromChain(cmd, contextKeyVerbose) {
		manager.SetCommandFlag(contextKeyVerbose, true)
	}
	if getBoolFlagFromChain(cmd, contextKeyQuiet) {
		manager.SetCommandFlag(contextKeyQuiet, true)
	}
	if profileFlag := getFlagFromChain(cmd, "context"); profileFlag != nil {
		if profile := profileFlag.Value.String(); profile != emptyValue {
			manager.SetCommandFlag(contextKeyProfile, profile)
		}
	}

	// Load and apply context
	return manager.LoadContext(initCtx)
}

// ResolveProjectRoot returns the project root using a single, explicit precedence so callers
// are always sure which root is in use. Use this everywhere project root is needed.
//
// Terminology: "Workspace root" = topmost directory containing .zqk (where .zqk/current_root lives).
// "Project root" = the directory used for this run (may be workspace root or a nested root).
//
// Precedence:
//  1. ZQK_PROJECT_ROOT — explicit production root (scripting, multi-repo)
//  2. ZQK_TEST_ROOT — explicit test root (test isolation)
//  3. Persisted current root — workspace's .zqk/current_root (see PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md)
//  4. findProjectRoot(startPath) — CWD-based discovery (walk up; nearest .zqk after filtering)
//
// When either env var is set, it is used and CWD/persisted are skipped for that process.
func ResolveProjectRoot(startPath string) string {
	return paths.ResolveProjectRoot(startPath)
}

// FindWorkspaceRoot returns the nearest directory (walking up from startPath) that contains
// .zqk. This is the workspace root: the single top-level directory where we store
// .zqk/current_root for the persisted project root. We use .zqk only (not go.mod) so that
// non-Go projects (Dart, Java, Python, etc.) have a consistent, language-agnostic marker.
func FindWorkspaceRoot(startPath string) string {
	return paths.FindWorkspaceRoot(startPath)
}

// readPersistedCurrentRoot reads workspace's .zqk/current_root and returns the project root path if valid.
func readPersistedCurrentRoot(workspaceRoot string) string {
	return paths.ReadPersistedCurrentRoot(workspaceRoot)
}

// WritePersistedCurrentRoot writes workspace's .zqk/current_root with the absolute project root path.
// projectRoot must be under workspaceRoot and valid (IsValidProjectRoot). Caller should stop scheduler
// for a previous root before switching; see PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md.
func WritePersistedCurrentRoot(workspaceRoot, projectRoot string) error {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return errfmt.Newf("project root path").Wrap(err)
	}
	if !paths.UnderProjectRoot(workspaceRoot, absRoot) {
		return errfmt.Errorf("project root must be under workspace (no escape)")
	}
	if !IsValidProjectRoot(absRoot) {
		return errfmt.Errorf("path is not a project root (must contain .zqk)")
	}
	dir := filepath.Join(workspaceRoot, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return errfmt.Newf("create .zqk").Wrap(err)
	}
	path := filepath.Join(dir, paths.CurrentRootFile)
	return fileutil.WriteSecureFile(path, []byte(absRoot+"\n"))
}

// IsValidProjectRoot returns true if dir contains .zqk (the project/workspace marker).
// We use .zqk only so that non-Go projects have a consistent, language-agnostic way to be recognized.
func IsValidProjectRoot(dir string) bool {
	return paths.IsValidProjectRoot(dir)
}

// ReadPersistedCurrentRoot is the exported form of readPersistedCurrentRoot for callers that need to
// read the current persisted root (e.g. use command to compare before overwrite).
func ReadPersistedCurrentRoot(workspaceRoot string) string {
	return paths.ReadPersistedCurrentRoot(workspaceRoot)
}

// OnCurrentRootSet is called when the persisted current root is written (e.g. after zqk use).
// Subscribers can register here to emit coordinator events, refresh caches, or notify other systems.
// Set by the CLI layer (e.g. root.go) to emit project_root_changed via the global coordinator
// so OperationalEventSubscriber listeners receive the event.
var OnCurrentRootSet func(workspaceRoot, newProjectRoot, previousProjectRoot string)

// NotifyCurrentRootSet invokes OnCurrentRootSet if set. Call after WritePersistedCurrentRoot
// so listeners (coordinator subscribers, other threads/systems) can react to the change.
func NotifyCurrentRootSet(workspaceRoot, newProjectRoot, previousProjectRoot string) {
	if OnCurrentRootSet != nil {
		OnCurrentRootSet(workspaceRoot, newProjectRoot, previousProjectRoot)
	}
}
