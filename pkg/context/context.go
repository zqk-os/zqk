package context

import (
	stdcontext "context"
	"io"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/when"
)

// MCPServerContext tracks MCP server serving state
// This is used by the logging framework to suppress output during active serving
// to prevent stdout pollution (Cursor may merge stderr into stdout for stdio-based MCP)
// This is in pkg/context to avoid import cycles between pkg/mcp and pkg/logging
type MCPServerContext struct {
	// serving tracks if any MCP server instance is actively serving requests
	serving int32 // atomic: 0 = not serving, 1 = actively serving
}

// globalMCPServerContext is the singleton instance tracking MCP server state
var globalMCPServerContext = &MCPServerContext{}

// GetMCPServerContext returns the global MCP server context
// This follows the pattern of other context types in this package
func GetMCPServerContext() *MCPServerContext {
	return globalMCPServerContext
}

// SetServing sets whether the MCP server is actively serving requests
// This should be called by the MCP server when it starts/stops serving
func (m *MCPServerContext) SetServing(serving bool) {
	when.When(func() bool { return serving }).Then(func() {
		atomic.StoreInt32(&m.serving, 1)
	}).OrElse(func() {
		atomic.StoreInt32(&m.serving, 0)
	}).Run()
}

// IsServing returns true if an MCP server is actively serving requests
// This allows the logging framework to suppress output during active serving
// to prevent stdout pollution
func (m *MCPServerContext) IsServing() bool {
	return atomic.LoadInt32(&m.serving) == 1
}

// SetMCPServerServing is a convenience function that sets the global MCP server serving state
// This maintains backward compatibility with the previous API
func SetMCPServerServing(serving bool) {
	globalMCPServerContext.SetServing(serving)
}

// IsMCPServerServing is a convenience function that checks the global MCP server serving state
// This maintains backward compatibility with the previous API
func IsMCPServerServing() bool {
	return globalMCPServerContext.IsServing()
}

// Account ID constants for system and common accounts
const (
	// SystemAccountID is the account ID used for system operations
	// This account is used when objects are created by automated processes
	SystemAccountID = "account:system"

	// FounderAccountID is the account ID for the founder account
	// This account has admin and founder roles
	FounderAccountID  = "account:founder"
	emptyContextValue = ""
)

// SecurityContext represents the actor performing an operation
// Implements ChainableContext for chain-based processing
type SecurityContext struct {
	AccountID               string   // account:username or ACC-###
	Roles                   []string // admin, developer, etc.
	Permissions             []string // read:*, write:backlog_item, etc.
	NamespaceID             string   // Isolated namespace for this context
	PersonaID               string   // The semantic persona of the agent
	ActiveVocabularySchemes []string // Active vocabulary schemes dictating graph visibility

	// Chain support
	precedence int
	depth      int
}

// NewSecurityContext creates a new SecurityContext
func NewSecurityContext(accountID string, roles, permissions []string) *SecurityContext {
	return &SecurityContext{
		AccountID:   accountID,
		Roles:       roles,
		Permissions: permissions,
		NamespaceID: "*",
		precedence:  PrecedenceDefault, // Default precedence
		depth:       DepthRoot,         // Default depth
	}
}

// NewSystemSecurityContext creates a SecurityContext for system operations
func NewSystemSecurityContext() *SecurityContext {
	return &SecurityContext{
		AccountID:   SystemAccountID,
		Roles:       []string{"admin"},
		Permissions: []string{"read:*", "write:*", "delete:*"},
		NamespaceID: "*",
		precedence:  PrecedenceSystem, // Highest precedence for system
		depth:       DepthRoot,
	}
}

// NewTestSecurityContext creates a SecurityContext for test operations
func NewTestSecurityContext() *SecurityContext {
	return &SecurityContext{
		AccountID:   "account:test-harness",
		Roles:       []string{"test"},
		Permissions: []string{"read:*", "write:*", "delete:*", "access:*"},
		NamespaceID: "*",
		precedence:  PrecedenceSystem,
		depth:       DepthRoot,
	}
}

// GetRoles returns the roles for this security context
func (s *SecurityContext) GetRoles() []string {
	return s.Roles
}

// GetPrecedence returns the precedence level for this context
func (s *SecurityContext) GetPrecedence() int {
	return s.precedence
}

// SetPrecedence sets the precedence level for this context
func (s *SecurityContext) SetPrecedence(precedence int) {
	s.precedence = precedence
}

// GetDepth returns the depth of this context in the hierarchy
func (s *SecurityContext) GetDepth() int {
	return s.depth
}

// SetDepth sets the depth of this context in the hierarchy
func (s *SecurityContext) SetDepth(depth int) {
	s.depth = depth
}

// Validate performs validation on this context node
func (s *SecurityContext) Validate() []ValidationError {
	var errors []ValidationError
	if s.AccountID == emptyContextValue {
		errors = append(errors, ValidationError{
			Field:   "AccountID",
			Message: "account ID cannot be empty",
			Context: "SecurityContext",
		})
	}
	return errors
}

// Merge merges this context into the target context
func (s *SecurityContext) Merge(target ChainableContext) ChainableContext {
	if target == nil {
		return s
	}

	// If target is also a SecurityContext, prefer the one with higher precedence
	if targetCtx, ok := target.(*SecurityContext); ok {
		if s.precedence < targetCtx.precedence {
			return s // This context has higher precedence
		}
		return targetCtx // Target has higher precedence
	}

	// Different context types - return this one
	return s
}

// securityContextKey is a private type for context keys to avoid collisions
type securityContextKey struct{}

// WithSecurityContext adds SecurityContext to the Go context
func WithSecurityContext(ctx stdcontext.Context, securityCtx *SecurityContext) stdcontext.Context {
	if securityCtx == nil {
		return ctx
	}
	return stdcontext.WithValue(ctx, securityContextKey{}, securityCtx)
}

// GetSecurityContext retrieves SecurityContext from the Go context
// Returns nil if no SecurityContext is present
func GetSecurityContext(ctx stdcontext.Context) *SecurityContext {
	if securityCtx, ok := ctx.Value(securityContextKey{}).(*SecurityContext); ok {
		return securityCtx
	}
	return nil
}

// NewSystemContext creates a context.Context with system security context
// Use this instead of context.Background() for system operations
// This ensures proper context propagation, cancellation, and security context
func NewSystemContext() stdcontext.Context {
	return WithSecurityContext(stdcontext.Background(), NewSystemSecurityContext())
}

var (
	globalStorageContextProvider = NewStorageContext
)

// RegisterStorageContextProvider registers a custom provider for StorageContext.
// This allows injection of specialized contexts without direct instantiation.
func RegisterStorageContextProvider(p func() *StorageContext) {
	globalStorageContextProvider = p
}

// GetStorageContext returns a new StorageContext using the registered provider.
func GetStorageContext() *StorageContext {
	if globalStorageContextProvider == nil {
		return NewStorageContext()
	}
	return globalStorageContextProvider()
}

// StorageContext represents context-specific parameters for storage operations
// These parameters can be set via context profiles (e.g., pagination context)
// Implements ChainableContext for chain-based processing
type StorageContext struct {
	MaxPageSize     int  // Maximum items per page (0 = no limit)
	DefaultPageSize int  // Default page size when not specified (0 = no default)
	EnableGrouping  bool // Whether grouping is enabled
	MaxGroupSize    int  // Maximum groups to return (0 = no limit)

	// Chain support
	precedence int
	depth      int
}

// NewStorageContext creates a new StorageContext with default values
func NewStorageContext() *StorageContext {
	return &StorageContext{
		MaxPageSize:     0,
		DefaultPageSize: 0,
		EnableGrouping:  false,
		MaxGroupSize:    0,
		precedence:      PrecedenceDefault, // Default precedence
		depth:           DepthRoot,
	}
}

// NewPaginationStorageContext creates a StorageContext with pagination settings
func NewPaginationStorageContext(maxPageSize, defaultPageSize int) *StorageContext {
	return &StorageContext{
		MaxPageSize:     maxPageSize,
		DefaultPageSize: defaultPageSize,
		EnableGrouping:  false,
		MaxGroupSize:    0,
		precedence:      PrecedenceProject, // Higher precedence for explicit settings
		depth:           DepthRoot,
	}
}

// NewGroupingStorageContext creates a StorageContext with grouping enabled
func NewGroupingStorageContext(maxGroupSize int) *StorageContext {
	return &StorageContext{
		MaxPageSize:     0,
		DefaultPageSize: 0,
		EnableGrouping:  true,
		MaxGroupSize:    maxGroupSize,
		precedence:      PrecedenceProject, // Higher precedence for explicit settings
		depth:           DepthRoot,
	}
}

// GetPrecedence returns the precedence level for this context
func (s *StorageContext) GetPrecedence() int {
	return s.precedence
}

// SetPrecedence sets the precedence level for this context
func (s *StorageContext) SetPrecedence(precedence int) {
	s.precedence = precedence
}

// GetDepth returns the depth of this context in the hierarchy
func (s *StorageContext) GetDepth() int {
	return s.depth
}

// SetDepth sets the depth of this context in the hierarchy
func (s *StorageContext) SetDepth(depth int) {
	s.depth = depth
}

// Validate performs validation on this context node
func (s *StorageContext) Validate() []ValidationError {
	var errors []ValidationError
	if s.MaxPageSize < 0 {
		errors = append(errors, ValidationError{
			Field:   "MaxPageSize",
			Message: "max page size cannot be negative",
			Context: "StorageContext",
		})
	}
	if s.DefaultPageSize < 0 {
		errors = append(errors, ValidationError{
			Field:   "DefaultPageSize",
			Message: "default page size cannot be negative",
			Context: "StorageContext",
		})
	}
	if s.MaxGroupSize < 0 {
		errors = append(errors, ValidationError{
			Field:   "MaxGroupSize",
			Message: "max group size cannot be negative",
			Context: "StorageContext",
		})
	}
	return errors
}

// Merge merges this context into the target context
func (s *StorageContext) Merge(target ChainableContext) ChainableContext {
	if target == nil {
		return s
	}

	// If target is also a StorageContext, merge values preferring higher precedence
	if targetCtx, ok := target.(*StorageContext); ok {
		result := &StorageContext{
			precedence: s.precedence,
			depth:      s.depth,
		}

		// Use values from context with higher precedence (lower precedence number)
		when.When(func() bool { return s.precedence < targetCtx.precedence }).Then(func() {
			result.MaxPageSize = s.MaxPageSize
			result.DefaultPageSize = s.DefaultPageSize
			result.EnableGrouping = s.EnableGrouping
			result.MaxGroupSize = s.MaxGroupSize
		}).OrElse(func() {
			result.MaxPageSize = targetCtx.MaxPageSize
			result.DefaultPageSize = targetCtx.DefaultPageSize
			result.EnableGrouping = targetCtx.EnableGrouping
			result.MaxGroupSize = targetCtx.MaxGroupSize
		}).Run()

		// Merge non-zero values (allow lower precedence to fill in defaults)
		if result.MaxPageSize == 0 && targetCtx.MaxPageSize != 0 {
			result.MaxPageSize = targetCtx.MaxPageSize
		}
		if result.DefaultPageSize == 0 && targetCtx.DefaultPageSize != 0 {
			result.DefaultPageSize = targetCtx.DefaultPageSize
		}
		if result.MaxGroupSize == 0 && targetCtx.MaxGroupSize != 0 {
			result.MaxGroupSize = targetCtx.MaxGroupSize
		}

		return result
	}

	// Different context types - return this one
	return s
}

// CliInitializationContext represents context for CLI initialization
// Bundles all initialization state to avoid one-off conditional checks
// Implements ContextNode for chain-based processing
type CliInitializationContext struct {
	ProjectRoot string // Resolved project root (never empty, defaults to ".")
	// Additional initialization state can be added here as needed

	// Chain support
	precedence int
	depth      int
}

// NewCliInitializationContext creates a new CliInitializationContext
// Resolves project root with fallback logic - always returns a valid context
func NewCliInitializationContext(findProjectRoot func(string) string, startDir string) *CliInitializationContext {
	projectRoot := findProjectRoot(startDir)
	if projectRoot == emptyContextValue {
		projectRoot = "."
	}
	return &CliInitializationContext{
		ProjectRoot: projectRoot,
		precedence:  PrecedenceDefault, // Default precedence
		depth:       DepthRoot,         // Default depth (root level)
	}
}

// GetProjectRoot returns the resolved project root (never empty)
func (c *CliInitializationContext) GetProjectRoot() string {
	return c.ProjectRoot
}

// GetPrecedence returns the precedence level for this context
// Lower values = higher precedence (processed first)
func (c *CliInitializationContext) GetPrecedence() int {
	return c.precedence
}

// SetPrecedence sets the precedence level for this context
func (c *CliInitializationContext) SetPrecedence(precedence int) {
	c.precedence = precedence
}

// GetDepth returns the depth of this context in the hierarchy
// 0 = root level, 1+ = nested levels
func (c *CliInitializationContext) GetDepth() int {
	return c.depth
}

// SetDepth sets the depth of this context in the hierarchy
func (c *CliInitializationContext) SetDepth(depth int) {
	c.depth = depth
}

// Validate performs validation on this context node
// Returns validation errors if any
func (c *CliInitializationContext) Validate() []ValidationError {
	var errors []ValidationError
	if c.ProjectRoot == emptyContextValue {
		errors = append(errors, ValidationError{
			Field:   "ProjectRoot",
			Message: "project root cannot be empty",
			Context: "CliInitializationContext",
		})
	}
	return errors
}

// Merge merges this context into the target context
// Used during breadth-first processing
func (c *CliInitializationContext) Merge(target ChainableContext) ChainableContext {
	// For initialization context, we typically want to keep the source (this context)
	// as it has the resolved project root
	if target == nil {
		return c
	}

	// If target is also a CliInitializationContext, prefer the one with higher precedence
	if targetCtx, ok := target.(*CliInitializationContext); ok {
		if c.precedence < targetCtx.precedence {
			return c // This context has higher precedence
		}
		return targetCtx // Target has higher precedence
	}

	// Different context types - return this one
	return c
}

// forceLifecycleOverrideKey is the context key for forcing lifecycle transition override (ITEM-621).
// When set, storage update skips lifecycle transition validation so status can be changed regardless of allowed transitions.
type forceLifecycleOverrideKey struct{}

// WithForceLifecycleOverride marks the context so that update operations skip lifecycle transition validation.
// Used when the user passes --force on "zqk object update <id> --field status=..." to allow any status change.
func WithForceLifecycleOverride(ctx stdcontext.Context) stdcontext.Context {
	return stdcontext.WithValue(ctx, forceLifecycleOverrideKey{}, true)
}

// GetForceLifecycleOverride returns true if the context has force lifecycle override set.
func GetForceLifecycleOverride(ctx stdcontext.Context) bool {
	if v, ok := ctx.Value(forceLifecycleOverrideKey{}).(bool); ok {
		return v
	}
	return false
}

// commandOutputWriterKey is the context key for an optional command output writer (e.g. test buffer).
type commandOutputWriterKey struct{}

// WithCommandOutputWriter attaches an io.Writer to the context so logging.GetCommandOutputWriter
// and CLI WriteOutput use it instead of os.Stdout. Used by tests to capture JSON/YAML output.
func WithCommandOutputWriter(ctx stdcontext.Context, w io.Writer) stdcontext.Context {
	return stdcontext.WithValue(ctx, commandOutputWriterKey{}, w)
}

// GetCommandOutputWriterFromContext returns the context's command output writer if set.
func GetCommandOutputWriterFromContext(ctx stdcontext.Context) (io.Writer, bool) {
	if ctx == nil {
		return nil, false
	}
	w, ok := ctx.Value(commandOutputWriterKey{}).(io.Writer)
	return w, ok
}

// stdinReaderKey is the context key for an optional stdin reader (e.g. test payload).
type stdinReaderKey struct{}

// WithStdinReader attaches an io.Reader to the context so callback/notify can read
// from it instead of os.Stdin. Used by tests to avoid global os.Stdin races.
func WithStdinReader(ctx stdcontext.Context, r io.Reader) stdcontext.Context {
	return stdcontext.WithValue(ctx, stdinReaderKey{}, r)
}

// GetStdinReaderFromContext returns the context's stdin reader if set.
func GetStdinReaderFromContext(ctx stdcontext.Context) (io.Reader, bool) {
	if ctx == nil {
		return nil, false
	}
	r, ok := ctx.Value(stdinReaderKey{}).(io.Reader)
	return r, ok
}

// lifecycleProjectRootKey is the context key for project root when invoking lifecycle hooks from storage.
// Storage attaches this so handlers (e.g. lifecycle updater) can resolve storage by project root.
type lifecycleProjectRootKey struct{}

// listCountSlotHeldKey is the context key used to track if a list/count concurrency slot is held.
// Prevents deadlocks during nested storage operations.
type listCountSlotHeldKey struct{}

// WithLifecycleProjectRoot attaches the project root to ctx for lifecycle hook handlers.
func WithLifecycleProjectRoot(ctx stdcontext.Context, projectRoot string) stdcontext.Context {
	if ctx == nil || projectRoot == emptyContextValue {
		return ctx
	}
	return stdcontext.WithValue(ctx, lifecycleProjectRootKey{}, projectRoot)
}

// WithListCountSlotHeld attaches a flag to the context indicating that a list/count slot is held.
func WithListCountSlotHeld(ctx stdcontext.Context) stdcontext.Context {
	if ctx == nil {
		return stdcontext.Background()
	}
	return stdcontext.WithValue(ctx, listCountSlotHeldKey{}, true)
}

// HasListCountSlotHeld returns true if a list/count slot is already held in the context.
func HasListCountSlotHeld(ctx stdcontext.Context) bool {
	if ctx == nil {
		return false
	}
	if b, ok := ctx.Value(listCountSlotHeldKey{}).(bool); ok {
		return b
	}
	return false
}

// GetLifecycleProjectRoot returns the project root from ctx if set (by storage when invoking lifecycle hooks).
func GetLifecycleProjectRoot(ctx stdcontext.Context) string {
	if ctx == nil {
		return emptyContextValue
	}
	if s, ok := ctx.Value(lifecycleProjectRootKey{}).(string); ok {
		return s
	}
	return emptyContextValue
}

// bypassCacheKey is the context key for bypassing caching storage.
type bypassCacheKey struct{}

// WithBypassCache marks the context so that storage operations bypass caching.
func WithBypassCache(ctx stdcontext.Context) stdcontext.Context {
	if ctx == nil {
		return stdcontext.Background()
	}
	return stdcontext.WithValue(ctx, bypassCacheKey{}, true)
}

// GetBypassCache returns true if the context has bypass cache set.
func GetBypassCache(ctx stdcontext.Context) bool {
	if ctx == nil {
		return false
	}
	if v, ok := ctx.Value(bypassCacheKey{}).(bool); ok {
		return v
	}
	return false
}
