package cli

import (
	stdcontext "context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	clictx "github.com/zqk-os/zqk/internal/cli/context"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	processorContextKeyFormat                   = "format"
	processorContextKeyVerbose                  = "verbose"
	processorContextKeyQuiet                    = "quiet"
	processorContextKeyProfile                  = "profile"
	processorContextKeyProjectRoot              = "project_root"
	processorContextKeyPriorityPlan             = objects.KindPriorityPlan
	processorContextKeyWorkstream               = objects.KindWorkstream
	processorContextKeyMilestone                = objects.KindMilestone
	processorContextKeyStorage                  = "storage"
	processorContextKeyStorageMaxPageSize       = "storage.max_page_size"
	processorContextKeyStorageDefaultPageSize   = "storage.default_page_size"
	processorContextKeyStorageEnableGrouping    = "storage.enable_grouping"
	processorContextKeyStorageMaxGroupSize      = "storage.max_group_size"
	processorContextKeyStorageMaxPageSizeShort  = "max_page_size"
	processorContextKeyStorageDefaultPageShort  = "default_page_size"
	processorContextKeyStorageEnableGroupShort  = "enable_grouping"
	processorContextKeyStorageMaxGroupSizeShort = "max_group_size"
	processorPrecedenceOrderKey                 = "precedence_order"
	processorCurrentFormatKey                   = "current_format"
	processorCurrentVerboseKey                  = "current_verbose"
	processorCurrentQuietKey                    = "current_quiet"
	processorCurrentProfileKey                  = "current_profile"
	// Matches pkg/scheduler.LogEventCacheInvalidationJobCompleted wire (POL-CODE-007); do not import pkg/scheduler here (import cycle).
	processorLogMsgCacheInvalidationCompleted = "cache_invalidation_job_completed"
	processorLogMsgCacheFreshnessCompleted    = "Cache freshness check completed"
	processorLogFieldCount                    = "count"
	processorLogFieldCleanedCount             = "cleaned_count"
	processorLogFieldReason                   = "reason"
	processorLogFieldState                    = "state"
	processorLogFieldDuration                 = "duration"
	processorLogFieldTriggerOperation         = "trigger_operation"
	processorRoleAdmin                        = "admin"
)

// Processor provides a unified interface for CLI operations with context-aware processing.
//
// Single Context Principle:
//
//	Everything is derivable from a single starting context. The processor holds one fully-processed
//	context that has already merged all layers (system, user, project, command). If you need variations,
//	use DeriveContext(), WithFormat(), WithStorageSettings(), etc. to create new contexts derived
//	from the starting context.
//
// Context Precedence Order (lowest to highest):
//  1. System Defaults - Built-in defaults (e.g., format="table", verbose=false)
//  2. User Config - ~/.zqk/config/config.yaml (user-level preferences)
//  3. Project Config - config/zqk.yaml (project-specific settings)
//  4. Command Flags - Command-line arguments (highest precedence)
//
// The processor respects this precedence when interpreting context values.
// For example, if a project config sets format="json" but a command flag sets --format=yaml,
// the final format will be "yaml" (command flags override project config).
type Processor struct {
	cliCtx       *Context
	storage      storage.ObjectStorageProvider
	secCtx       *pkgctx.SecurityContext
	logger       *logging.EventLogger
	projectRoot  string
	storageCtx   *pkgctx.StorageContext
	operationCtx stdcontext.Context
}

// resolveProcessorProjectRoot returns the project root used for storage initialization.
// When [Context.ProjectRoot] is set (explicit --project-root, tests via [ContextForProjectRoot]),
// that path wins so storage matches file walks and resolver output; otherwise [ResolveProjectRoot]
// is used from the current directory (same precedence as workspace discovery and env overrides).
func resolveProcessorProjectRoot(cliCtx *Context) string {
	var explicit string
	if cliCtx != nil && cliCtx.Context != nil {
		explicit = cliCtx.ProjectRoot
	}
	if explicit == emptyValue || explicit == "." {
		initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
		return initCtx.GetProjectRoot()
	}
	if abs, err := filepath.Abs(explicit); err == nil {
		return abs
	}
	return explicit
}

// NewProcessor creates a new context processor from a command.
// This centralizes context interpretation, security checks, and system object handling.
//
// The processor loads context with proper precedence:
//  1. System defaults are loaded first
//  2. User config (~/.zqk/config/config.yaml) is loaded and merged
//  3. Project config (config/zqk.yaml) is loaded and merged
//  4. Command flags are extracted and merged (highest precedence)
//
// All context values respect this precedence order.
func NewProcessor(cmd *cobra.Command) (*Processor, error) {
	// Get CLI context (already loaded in PersistentPreRunE with proper precedence)
	cliCtx := GetContext(cmd)
	if cliCtx == nil {
		return nil, errfmt.Errorf("failed to get context")
	}

	projectRoot := resolveProcessorProjectRoot(cliCtx)

	// Reuse storage from PreRunE when present (object/internal subcommands get one shared init per run).
	// Fallback: use global cache so we never create a new storage instance (avoids blocking init and extra load).
	var storageProvider storage.ObjectStorageProvider
	if p := GetStorageProvider(cmd.Context()); p != nil {
		storageProvider = p.(storage.ObjectStorageProvider)
	} else {
		var err error
		storageProvider, err = storage.GetGlobalStorageProviderCache().GetOrCreate(cmd.Context(), projectRoot)
		if err != nil {
			return nil, errfmt.Newf("failed to get storage").Wrap(err)
		}
		// Wire synonym resolver when we create here (PreRunE already does this when it creates)
		resolver := objects.GetGlobalSynonymResolver()
		if resolver != nil {
			resolver.SetSynonymLoader(storage.NewStorageSynonymLoader(storageProvider))
		}
	}

	// Create security context (use context from AuthMiddleware if present, otherwise fail-closed guest context)
	secCtx := pkgctx.GetSecurityContext(cmd.Context())
	if secCtx == nil {
		if testing.Testing() || strings.HasSuffix(os.Args[0], ".test") {
			secCtx = pkgctx.NewTestSecurityContext()
		} else {
			secCtx = pkgctx.NewGuestSecurityContext()
		}
	}
	// Get logger from command context
	logger := logging.GetLoggerFromContext(cmd.Context())

	// Create storage context from CLI context (respects precedence for storage settings)
	storageCtx := cliCtx.GetStorageContext()

	// Wrap the storage provider in the Semantic Storage Decorator to enforce
	// Persona-Aware Context Filtering rules invisibly across the CLI and MCP
	semanticStorage := &SemanticStorageDecorator{
		ObjectStorageProvider: storageProvider,
	}

	return &Processor{
		cliCtx:       cliCtx,
		storage:      semanticStorage,
		secCtx:       secCtx,
		logger:       logger,
		projectRoot:  projectRoot,
		storageCtx:   storageCtx,
		operationCtx: cmd.Context(),
	}, nil
}

// Context returns the CLI context wrapper.
func (p *Processor) Context() *Context {
	return p.cliCtx
}

// CheckGovernorApproval enforces the Governor pattern for high-stakes commands.
// It checks if the command requires approval. If it does, and no event-id is provided,
// it creates a PROPOSED audit_event and returns an error instructing the user to sign it.
// If an event-id is provided, it verifies the APPROVED edge exists in the graph.
func (p *Processor) CheckGovernorApproval(cmd *cobra.Command) error {
	req := GetCommandRequirements(cmd.Root(), os.Args[1:])
	if !req.RequiresGovernorApproval {
		return nil // No approval required
	}

	var flagsBag clipkg.FlagBag
	eventID := flagsBag.String(cmd, "event-id")
	if flagsBag.Err() != nil || eventID == "" {
		// Needs approval but no event ID provided.
		// 1. Create a PROPOSED event for this command.
		// Use AuditEvent as the base event type.
		proposedEvent := map[string]any{
			objects.FieldKeyKind:      "audit_event",
			objects.FieldKeyTitle:     fmt.Sprintf("Proposed: %s", cmd.CommandPath()),
			objects.FieldKeyOperation: cmd.CommandPath(),
			objects.FieldKeyEventType: "command_execution",
			objects.FieldKeyStatus:    "proposed",
			objects.FieldKeyMetadata: map[string]any{
				"args": os.Args[1:],
			},
		}

		err := p.storage.Create(p.operationCtx, p.secCtx, proposedEvent)
		if err != nil {
			return errfmt.Newf("failed to create proposed event").Wrap(err)
		}

		// 2. Create the PROPOSED edge if it's a Graph storage
		newID, _ := proposedEvent[objects.FieldKeyID].(string)

		// Helper to extract GraphObjectStorage
		var graphStorage *storage.GraphObjectStorage
		switch s := p.storage.(type) {
		case *storage.GraphObjectStorage:
			graphStorage = s
		case *storage.HybridObjectStorage:
			if g, ok := s.GetPrimary().(*storage.GraphObjectStorage); ok {
				graphStorage = g
			}
		case *storage.RoutingObjectStorage:
			sForKind := s.GetStorageFactory().GetStorageForKind("audit_event")
			if g, ok := sForKind.(*storage.GraphObjectStorage); ok {
				graphStorage = g
			} else if h, ok := sForKind.(*storage.HybridObjectStorage); ok {
				if g, ok := h.GetPrimary().(*storage.GraphObjectStorage); ok {
					graphStorage = g
				}
			}
		}

		if graphStorage != nil && newID != "" {
			_ = graphStorage.CreateProposedEdge(p.operationCtx, p.secCtx.AccountID, newID)
		}

		return errfmt.Errorf("Governor approval required. Created proposed event %s. Wait for human to run 'zqk sign %s', then re-run this command with --event-id=%s", newID, newID, newID)
	}

	// Verify the APPROVED edge exists
	var graphStorage *storage.GraphObjectStorage
	switch s := p.storage.(type) {
	case *storage.GraphObjectStorage:
		graphStorage = s
	case *storage.HybridObjectStorage:
		if g, ok := s.GetPrimary().(*storage.GraphObjectStorage); ok {
			graphStorage = g
		}
	case *storage.RoutingObjectStorage:
		sForKind := s.GetStorageFactory().GetStorageForKind("audit_event")
		if g, ok := sForKind.(*storage.GraphObjectStorage); ok {
			graphStorage = g
		} else if h, ok := sForKind.(*storage.HybridObjectStorage); ok {
			if g, ok := h.GetPrimary().(*storage.GraphObjectStorage); ok {
				graphStorage = g
			}
		}
	}

	if graphStorage == nil {
		return errfmt.Errorf("graph backend required for Governor approval verification")
	}

	approved, err := graphStorage.VerifyProvenance(p.operationCtx, p.secCtx, eventID)
	if err != nil {
		return errfmt.Newf("failed to verify governor approval").Wrap(err)
	}

	if !approved {
		return errfmt.Errorf("Governor approval pending. Event %s has not been signed. Run 'zqk sign %s' first", eventID, eventID)
	}

	return nil
}

// Storage returns the storage provider
func (p *Processor) Storage() storage.ObjectStorageProvider {
	return p.storage
}

// SecurityContext returns the security context
func (p *Processor) SecurityContext() *pkgctx.SecurityContext {
	return p.secCtx
}

// Logger returns the logger
func (p *Processor) Logger() *logging.EventLogger {
	return p.logger
}

// ProjectRoot returns the project root
func (p *Processor) ProjectRoot() string {
	return p.projectRoot
}

// StorageContext returns the storage context
func (p *Processor) StorageContext() *pkgctx.StorageContext {
	return p.storageCtx
}

// StorageFactory returns a new storage factory for the current project root.
// Note: This creates a new factory instance each time.
func (p *Processor) StorageFactory() *storage.StorageFactory {
	factory, _ := storage.NewStorageFactory(p.operationCtx, p.projectRoot)
	return factory
}

// OperationContext returns the operation context
func (p *Processor) OperationContext() stdcontext.Context {
	return p.operationCtx
}

// Format returns the output format
func (p *Processor) Format() OutputFormat {
	return p.cliCtx.Format
}

// IsVerbose returns whether verbose mode is enabled
func (p *Processor) IsVerbose() bool {
	return p.cliCtx.Verbose
}

// IsQuiet returns whether quiet mode is enabled
func (p *Processor) IsQuiet() bool {
	return p.cliCtx.Quiet
}

// ValidateObject performs validation on an object including:
// - Spec validation
// - Lifecycle validation
// - Reference validation
// Note: This is a convenience method. Full validation happens in storage layer during Create/Update.
// This method can be used for pre-validation checks.
func (p *Processor) ValidateObject(obj map[string]any, kind, currentState string) error {
	// Validation is handled by the storage layer during Create/Update operations
	// This method provides a way to check validation before attempting operations
	// For now, we rely on storage layer validation
	// In the future, we could add pre-validation here if needed

	// Basic checks
	if kind == emptyValue {
		return errfmt.Errorf("object kind is required")
	}
	if obj == nil {
		return errfmt.Errorf("object cannot be nil")
	}

	// The storage layer will perform full validation (spec, lifecycle, references)
	// when Create/Update is called

	return nil
}

// CheckPermission checks if the current security context has permission for an operation
func (p *Processor) CheckPermission(operation, kind string) error {
	// This is a convenience method - actual permission checking happens in storage layer
	// But we can provide a way to check permissions before attempting operations
	// For now, system context has all permissions, but this can be extended
	return nil
}

// IsSystemObject checks if an object is a system object (built-in or internal)
func (p *Processor) IsSystemObject(obj map[string]any) bool {
	return storage.IsBuiltIn(obj)
}

// CanModifySystemObject checks if the current context can modify a system object
func (p *Processor) CanModifySystemObject(obj map[string]any) bool {
	if !p.IsSystemObject(obj) {
		return true // Non-system objects can always be modified (subject to permissions)
	}

	// System objects require admin role
	return slices.Contains(p.secCtx.Roles, processorRoleAdmin)
}

// GetStorageProvider returns the storage provider (for compatibility)
//
// Deprecated: Use Storage() instead
func (p *Processor) GetStorageProvider() storage.ObjectStorageProvider {
	return p.storage
}

// WithCLIOperation wraps the operation context with CLI operation marker
// This is required for delete operations and other sensitive operations
func (p *Processor) WithCLIOperation() stdcontext.Context {
	return storage.WithCLIOperation(p.operationCtx)
}

// GetContextLayers returns the context layer map for inspection
// This allows commands to see which layer provided which values
// Returns a map of ContextLayer -> config values for each precedence layer
func (p *Processor) GetContextLayers() map[clictx.ContextLayer]map[string]any {
	return p.cliCtx.GetLayers()
}

// GetEffectiveValue returns the effective value for a context key after precedence is applied
// This is the final value that will be used, respecting all precedence layers
func (p *Processor) GetEffectiveValue(key string) (any, bool) {
	switch key {
	case processorContextKeyFormat:
		return string(p.cliCtx.Format), true
	case processorContextKeyVerbose:
		return p.cliCtx.Verbose, true
	case processorContextKeyQuiet:
		return p.cliCtx.Quiet, true
	case processorContextKeyProfile:
		return p.cliCtx.Profile, true
	case processorContextKeyProjectRoot:
		return p.cliCtx.ProjectRoot, true
	case processorContextKeyPriorityPlan:
		return p.cliCtx.PriorityPlan, true
	case processorContextKeyWorkstream:
		return p.cliCtx.Workstream, true
	case processorContextKeyMilestone:
		return p.cliCtx.Milestone, true
	case processorContextKeyStorageMaxPageSize:
		return p.cliCtx.StorageMaxPageSize, true
	case processorContextKeyStorageDefaultPageSize:
		return p.cliCtx.StorageDefaultPageSize, true
	case processorContextKeyStorageEnableGrouping:
		return p.cliCtx.StorageEnableGrouping, true
	case processorContextKeyStorageMaxGroupSize:
		return p.cliCtx.StorageMaxGroupSize, true
	default:
		return nil, false
	}
}

// GetContextPrecedenceInfo returns information about context precedence for debugging
// This helps understand which layer provided which values
func (p *Processor) GetContextPrecedenceInfo() map[string]string {
	info := make(map[string]string)

	// Document the precedence order
	info[processorPrecedenceOrderKey] = "system -> user -> project -> command"
	info[processorCurrentFormatKey] = string(p.cliCtx.Format)
	info[processorCurrentVerboseKey] = fmt.Sprintf("%v", p.cliCtx.Verbose)
	info[processorCurrentQuietKey] = fmt.Sprintf("%v", p.cliCtx.Quiet)
	info[processorCurrentProfileKey] = p.cliCtx.Profile
	info[processorContextKeyProjectRoot] = p.cliCtx.ProjectRoot

	return info
}

// DeriveContext creates a new context derived from the processor's current context with overrides
// This is the preferred way to create context variations - everything derives from the starting context
func (p *Processor) DeriveContext(overrides map[string]any) *Context {
	// Derive from the internal context
	derived := p.cliCtx.Context.Derive(overrides)

	// Convert back to cli.Context
	return &Context{
		Context: derived,
		Format:  OutputFormat(derived.Format),
	}
}

// WithFormat creates a new processor with a different format, derived from the current context
func (p *Processor) WithFormat(format string) *Processor {
	derivedCtx := p.DeriveContext(map[string]any{processorContextKeyFormat: format})
	return &Processor{
		cliCtx:       derivedCtx,
		storage:      p.storage,
		secCtx:       p.secCtx,
		logger:       p.logger,
		projectRoot:  p.projectRoot,
		storageCtx:   derivedCtx.GetStorageContext(),
		operationCtx: p.operationCtx,
	}
}

// WithStorageSettings creates a new processor with different storage settings
func (p *Processor) WithStorageSettings(maxPageSize, defaultPageSize int, enableGrouping bool, maxGroupSize int) *Processor {
	derivedCtx := p.DeriveContext(map[string]any{
		processorContextKeyStorage: map[string]any{
			processorContextKeyStorageMaxPageSizeShort:  maxPageSize,
			processorContextKeyStorageDefaultPageShort:  defaultPageSize,
			processorContextKeyStorageEnableGroupShort:  enableGrouping,
			processorContextKeyStorageMaxGroupSizeShort: maxGroupSize,
		},
	})
	return &Processor{
		cliCtx:       derivedCtx,
		storage:      p.storage,
		secCtx:       p.secCtx,
		logger:       p.logger,
		projectRoot:  p.projectRoot,
		storageCtx:   derivedCtx.GetStorageContext(),
		operationCtx: p.operationCtx,
	}
}

// BuildContextSequential builds a context from multiple contexts processed sequentially
// NOTE: This is for advanced use cases. Prefer DeriveContext() for normal operations.
// Each context in the list overrides values from previous contexts
func (p *Processor) BuildContextSequential(contexts []*Context) (*Context, error) {
	// Convert cli.Context to context.Context
	ctxContexts := make([]*clictx.Context, 0, len(contexts))
	for _, ctx := range contexts {
		ctxContexts = append(ctxContexts, ctx.Context)
	}

	processor := clictx.NewContextProcessor(clictx.ModeSequential)
	result, err := processor.ProcessSequential(ctxContexts)
	if err != nil {
		return nil, err
	}

	// Convert back to cli.Context
	return &Context{
		Context: result,
		Format:  OutputFormat(result.Format),
	}, nil
}

// BuildContextHierarchical builds a context from a hierarchical tree structure
// NOTE: This is for advanced use cases. Prefer DeriveContext() for normal operations.
// Child contexts inherit from parent and override parent values
func (p *Processor) BuildContextHierarchical(root *clictx.ContextNode) (*Context, error) {
	processor := clictx.NewContextProcessor(clictx.ModeHierarchical)
	result, err := processor.ProcessHierarchical(root)
	if err != nil {
		return nil, err
	}

	// Convert back to cli.Context
	return &Context{
		Context: result,
		Format:  OutputFormat(result.Format),
	}, nil
}

// BuildContextHybrid builds a context with both sequential and hierarchical aspects
// NOTE: This is for advanced use cases. Prefer DeriveContext() for normal operations.
// Siblings are processed sequentially, children inherit hierarchically
func (p *Processor) BuildContextHybrid(root *clictx.ContextNode) (*Context, error) {
	processor := clictx.NewContextProcessor(clictx.ModeHybrid)
	result, err := processor.ProcessHybrid(root)
	if err != nil {
		return nil, err
	}

	// Convert back to cli.Context
	return &Context{
		Context: result,
		Format:  OutputFormat(result.Format),
	}, nil
}

// NewContextBuilder creates a new context builder for flexible context assembly
// NOTE: This is for advanced use cases. For normal operations, use DeriveContext() on the processor.
func NewContextBuilder(mode clictx.ProcessingMode) *clictx.ContextBuilder {
	return clictx.NewContextBuilder(mode)
}

// CacheInvalidationHandler is a function type for performing cache invalidation
// This allows the processor to delegate cache operations to the appropriate implementation
// without creating circular dependencies
type CacheInvalidationHandler func(ids []string, projectRoot string) int

// cacheInvalidationHandler is the global handler for cache invalidation
// It can be set by packages that implement cache operations (e.g., cmd/zqk/system)
//
//nolint:unused // Part of API - registered but may not be called in all code paths
var cacheInvalidationHandler CacheInvalidationHandler

// RegisterCacheInvalidationHandler registers a handler for cache invalidation operations
// This should be called by packages that implement cache operations (e.g., cmd/zqk/system)
// to provide the processor with a way to perform cache invalidation
func RegisterCacheInvalidationHandler(handler CacheInvalidationHandler) {
	cacheInvalidationHandler = handler
}

// CacheFreshnessHandler is a function type for performing cache freshness checks
// This allows the processor to delegate cache freshness operations to the appropriate implementation
// without creating circular dependencies
type CacheFreshnessHandler func(projectRoot, reason, triggerOperation string, affectedKinds []string) int

// cacheFreshnessHandler is the global handler for cache freshness checks
// It can be set by packages that implement cache operations (e.g., cmd/zqk/system)
//
//nolint:unused // Part of API - registered but may not be called in all code paths
var cacheFreshnessHandler CacheFreshnessHandler

// RegisterCacheFreshnessHandler registers a handler for cache freshness operations
// This should be called by packages that implement cache operations (e.g., cmd/zqk/system)
// to provide the processor with a way to perform cache freshness checks
func RegisterCacheFreshnessHandler(handler CacheFreshnessHandler) {
	cacheFreshnessHandler = handler
}

// InvalidateCache interprets a CacheInvalidationContext and performs bulk cache invalidation
// This method understands how to interpret the context object and perform the cache operation
// It handles:
//   - Validating the context
//   - Resolving project root (from context or processor)
//   - Processing through the listener pipeline (async if configured)
//
// Returns the number of cache entries invalidated
// If async processing is enabled, this returns immediately and processing happens in background
func (p *Processor) InvalidateCache(ctx *pkgctx.CacheInvalidationContext) (int, error) {
	if ctx == nil {
		return 0, errfmt.Errorf("cache invalidation context is required")
	}

	if len(ctx.IDs) == 0 {
		// No IDs to invalidate
		return 0, nil
	}

	// Use project root from context if provided, otherwise use processor's project root
	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = p.projectRoot
	}
	if projectRoot == emptyValue {
		return 0, errfmt.Errorf("project root is required for cache invalidation")
	}

	// Set the operation context for cancellation
	ctx.WithContext(p.operationCtx)

	// Process through the listener pipeline (triggers async listeners if registered)
	resultChan := pkgctx.ProcessContext(ctx)

	// Wait for result (with timeout if needed)
	result, err := pkgctx.WaitForResult(resultChan, 0) // 0 = no timeout, wait indefinitely
	if err != nil {
		return 0, errfmt.Newf("failed to process cache invalidation").Wrap(err)
	}

	// Check for errors
	if result.FinalError != nil {
		return 0, result.FinalError
	}

	// Extract count from results
	var count int
	if countVal, ok := result.Results["cache_invalidation"]; ok {
		if c, ok := countVal.(int); ok {
			count = c
		}
	}

	// Log the operation if verbose
	if p.IsVerbose() && ctx.Reason != emptyValue {
		p.logger.WithFields(
			logging.Int(processorLogFieldCount, count),
			logging.String(processorLogFieldReason, ctx.Reason),
			logging.String(processorLogFieldState, string(result.State)),
			logging.String(processorLogFieldDuration, result.Duration.String()),
		).LogDebug(processorLogMsgCacheInvalidationCompleted)
	}

	return count, nil
}

// InvalidateCacheAsync processes cache invalidation asynchronously and returns immediately
// Returns a channel that will receive the result when processing completes
func (p *Processor) InvalidateCacheAsync(ctx *pkgctx.CacheInvalidationContext) <-chan *pkgctx.ProcessingResult {
	if ctx == nil {
		resultChan := make(chan *pkgctx.ProcessingResult, 1)
		resultChan <- &pkgctx.ProcessingResult{
			State:      pkgctx.StateFailed,
			FinalError: errfmt.Errorf("cache invalidation context is required"),
		}
		close(resultChan)
		return resultChan
	}

	// Set the operation context for cancellation
	ctx.WithContext(p.operationCtx)

	// Process through the listener pipeline (triggers async listeners if registered)
	return pkgctx.ProcessContext(ctx)
}

// CheckCacheFreshness interprets a CacheFreshnessContext and performs cache freshness validation
// This method understands how to interpret the context object and perform the cache operation
// It handles:
//   - Validating the context
//   - Resolving project root (from context or processor)
//   - Checking if cache freshness is enabled in the CLI context
//   - Processing through the listener pipeline (async if configured)
//
// Returns the number of stale cache entries cleaned
// If async processing is enabled, this returns immediately and processing happens in background
func (p *Processor) CheckCacheFreshness(ctx *pkgctx.CacheFreshnessContext) (int, error) {
	// Check if cache freshness is enabled in the CLI context
	// Default to true if not explicitly set
	if !p.cliCtx.CacheFreshnessEnabled {
		if p.IsVerbose() {
			p.logger.LogDebug("Cache freshness check skipped (disabled in context)")
		}
		return 0, nil
	}

	if ctx == nil {
		return 0, errfmt.Errorf("cache freshness context is required")
	}

	// Use project root from context if provided, otherwise use processor's project root
	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = p.projectRoot
	}
	if projectRoot == emptyValue {
		return 0, errfmt.Errorf("project root is required for cache freshness check")
	}

	// Set the operation context for cancellation
	ctx.WithContext(p.operationCtx)

	// Process through the listener pipeline (triggers async listeners if registered)
	resultChan := pkgctx.ProcessContext(ctx)

	// Wait for result (with timeout if needed)
	result, err := pkgctx.WaitForResult(resultChan, 0) // 0 = no timeout, wait indefinitely
	if err != nil {
		return 0, errfmt.Newf("failed to process cache freshness check").Wrap(err)
	}

	// Check for errors
	if result.FinalError != nil {
		return 0, result.FinalError
	}

	// Extract count from results
	var count int
	if countVal, ok := result.Results["cache_freshness"]; ok {
		if resultMap, ok := countVal.(map[string]any); ok {
			if c, ok := resultMap["cleaned_count"].(int); ok {
				count = c
			}
		}
	}

	// Log the operation if verbose
	if p.IsVerbose() && ctx.Reason != emptyValue {
		p.logger.WithFields(
			logging.Int(processorLogFieldCleanedCount, count),
			logging.String(processorLogFieldReason, ctx.Reason),
			logging.String(processorLogFieldTriggerOperation, ctx.TriggerOperation),
			logging.String(processorLogFieldState, string(result.State)),
			logging.String(processorLogFieldDuration, result.Duration.String()),
		).LogDebug(processorLogMsgCacheFreshnessCompleted)
	}

	return count, nil
}

// CheckCacheFreshnessAsync processes cache freshness check asynchronously and returns immediately
// Returns a channel that will receive the result when processing completes
func (p *Processor) CheckCacheFreshnessAsync(ctx *pkgctx.CacheFreshnessContext) <-chan *pkgctx.ProcessingResult {
	// Check if cache freshness is enabled in the CLI context
	// Default to true if not explicitly set
	if !p.cliCtx.CacheFreshnessEnabled {
		resultChan := make(chan *pkgctx.ProcessingResult, 1)
		resultChan <- &pkgctx.ProcessingResult{
			State:      pkgctx.StateCompleted,
			Results:    map[string]any{"cleaned_count": 0},
			FinalError: nil,
		}
		close(resultChan)
		return resultChan
	}

	if ctx == nil {
		resultChan := make(chan *pkgctx.ProcessingResult, 1)
		resultChan <- &pkgctx.ProcessingResult{
			State:      pkgctx.StateFailed,
			FinalError: errfmt.Errorf("cache freshness context is required"),
		}
		close(resultChan)
		return resultChan
	}

	// Use project root from context if provided, otherwise use processor's project root
	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = p.projectRoot
	}
	if projectRoot == emptyValue {
		resultChan := make(chan *pkgctx.ProcessingResult, 1)
		resultChan <- &pkgctx.ProcessingResult{
			State:      pkgctx.StateFailed,
			FinalError: errfmt.Errorf("project root is required for cache freshness check"),
		}
		close(resultChan)
		return resultChan
	}

	// Set the operation context for cancellation
	ctx.WithContext(p.operationCtx)

	// Process through the listener pipeline (triggers async listeners if registered)
	return pkgctx.ProcessContext(ctx)
}

// TriggerCacheFreshnessCheck is a convenience method that creates a CacheFreshnessContext
// and triggers a cache freshness check based on a storage operation
// This should be called after storage operations (Create, Update, Delete, Bulk operations)
// to ensure the cache stays fresh
func (p *Processor) TriggerCacheFreshnessCheck(triggerOperation string, affectedKinds []string) {
	// Check if cache freshness is enabled
	if !p.cliCtx.CacheFreshnessEnabled {
		return
	}

	// Check if this operation type should trigger a freshness check
	triggers := p.cliCtx.CacheFreshnessTriggers
	if len(triggers) > 0 {
		// Check if triggerOperation is in the list
		found := false
		for _, trigger := range triggers {
			if trigger == triggerOperation || trigger == "*" {
				found = true
				break
			}
		}
		if !found {
			// This operation type is not configured to trigger freshness checks
			return
		}
	}

	// Create freshness context
	freshnessCtx := pkgctx.NewCacheFreshnessContext(
		p.projectRoot,
		fmt.Sprintf("Storage operation: %s", triggerOperation),
		triggerOperation,
	).WithAffectedKinds(affectedKinds).WithContext(p.operationCtx)

	// Trigger async freshness check (non-blocking)
	_ = p.CheckCacheFreshnessAsync(freshnessCtx)
}

// WithProcessor wraps a cobra RunE function to automatically initialize and inject a Processor.
func WithProcessor(handler func(cmd *cobra.Command, args []string, proc *Processor) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		proc, err := NewProcessor(cmd)
		if err != nil {
			return errfmt.Errorf("failed to create processor: %w", err)
		}
		err = handler(cmd, args, proc)
		if err != nil {
			return Guard(cmd).Err(err).Return()
		}
		return nil
	}
}
