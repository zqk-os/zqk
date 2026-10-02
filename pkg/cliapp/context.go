package cli

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
)

// Context wraps the internal context with OutputFormat conversion
type Context struct {
	*context.Context
	Format OutputFormat // Converted from string
}

const (
	contextOverrideFormat  = "format"
	contextOverrideVerbose = "verbose"
	contextOverrideQuiet   = "quiet"
	contextOverrideProfile = "profile"
)

// withInnerContext runs fn with the embedded *context.Context when c is non-nil and c.Context
// is non-nil; otherwise returns zero. Centralizes the nil guard for wrapper methods—keep fn short
// so each method’s behavior stays visible at the call site. See docs/architecture/WRAPPER_INNER_CONTEXT_GUARD_PATTERN.md
// (glossary ); scripts/check-cli-inner-guard.sh enforces a single guard string.
func withInnerContext[T any](c *Context, fn func(*context.Context) T, zero T) T {
	if c == nil || c.Context == nil {
		return zero
	}
	return fn(c.Context)
}

// Derive creates a new context derived from this context with optional overrides
// This allows functions to work with a single starting context and derive variations
func (c *Context) Derive(overrides map[string]any) *Context {
	return withInnerContext(c, func(ic *context.Context) *Context {
		derived := ic.Derive(overrides)
		return &Context{
			Context: derived,
			Format:  OutputFormat(derived.Format),
		}
	}, nil)
}

// WithFormat creates a new context with a different format
func (c *Context) WithFormat(format string) *Context {
	return c.Derive(map[string]any{contextOverrideFormat: format})
}

// WithVerbose creates a new context with verbose enabled/disabled
func (c *Context) WithVerbose(verbose bool) *Context {
	return c.Derive(map[string]any{contextOverrideVerbose: verbose})
}

// WithQuiet creates a new context with quiet enabled/disabled
func (c *Context) WithQuiet(quiet bool) *Context {
	return c.Derive(map[string]any{contextOverrideQuiet: quiet})
}

// WithProfile creates a new context with a different profile
func (c *Context) WithProfile(profile string) *Context {
	return c.Derive(map[string]any{contextOverrideProfile: profile})
}

// GetStorageContext creates a StorageContext from the CLI context
func (c *Context) GetStorageContext() *pkgctx.StorageContext {
	return withInnerContext(c, func(ic *context.Context) *pkgctx.StorageContext {
		return &pkgctx.StorageContext{
			MaxPageSize:     ic.StorageMaxPageSize,
			DefaultPageSize: ic.StorageDefaultPageSize,
			EnableGrouping:  ic.StorageEnableGrouping,
			MaxGroupSize:    ic.StorageMaxGroupSize,
		}
	}, &pkgctx.StorageContext{})
}

// PathResolver returns a paths.PathResolver for the current project root (alias cache–aware resolution).
// See pkg/paths.PathResolver and docs/architecture/PATH_ALIAS_RESOLUTION.md.
func (c *Context) PathResolver() paths.PathResolver {
	return withInnerContext(c, func(ic *context.Context) paths.PathResolver {
		return ic.PathResolver()
	}, paths.NewPathResolver(""))
}

// WithPathResolver returns a derived context that uses r for PathResolver (tests); nil clears override.
func (c *Context) WithPathResolver(r paths.PathResolver) *Context {
	return withInnerContext(c, func(ic *context.Context) *Context {
		return &Context{
			Context: ic.WithPathResolver(r),
			Format:  c.Format,
		}
	}, nil)
}

// Re-export context types and functions for convenience
type ContextManager = context.ContextManager
type ContextLayer = context.ContextLayer

const (
	LayerSystem  = context.LayerSystem
	LayerUser    = context.LayerUser
	LayerProject = context.LayerProject
	LayerCommand = context.LayerCommand
)

// GetContextFromCommand extracts context from a cobra command
// initCtx provides CLI initialization context (project root, etc.) - always pass context objects, never extract values
func GetContextFromCommand(cmd *cobra.Command, initCtx *pkgctx.CliInitializationContext) (*Context, error) {
	ctx, err := context.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		return nil, err
	}

	// Convert string format to OutputFormat
	format := OutputFormat(ctx.Format)
	if format == emptyValue {
		format = FormatTable // Default
	}

	return &Context{
		Context: ctx,
		Format:  format,
	}, nil
}

// ResolveProjectRoot returns the project root with explicit precedence (ZQK_PROJECT_ROOT,
// ZQK_TEST_ROOT, then CWD-based discovery). Use this so the process is always sure which root is in use.
func ResolveProjectRoot(startPath string) string {
	return context.ResolveProjectRoot(startPath)
}

// ResolveCommandProjectRoot resolves and validates the project root from a cobra.Command's CLI context.
func ResolveCommandProjectRoot(cmd *cobra.Command) (string, error) {
	ctx := GetContext(cmd)
	if ctx == nil {
		return "", errfmt.Errorf("failed to get context")
	}
	projectRoot := ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root not found")
	}
	return projectRoot, nil
}

// NewContextManager creates a new context manager
func NewContextManager() *ContextManager {
	return context.NewContextManager()
}

// ContextForProjectRoot returns a minimal *Context for tests and integration fixtures:
// table output format and an embedded context with only ProjectRoot set.
// Chain WithProfile / WithFormat when tests need non-default profile or inner format string.
func ContextForProjectRoot(projectRoot string) *Context {
	return &Context{
		Context: &context.Context{ProjectRoot: projectRoot},
		Format:  FormatTable,
	}
}

// ContextForProjectAndProfile returns *Context with project root, profile, and table format.
// Equivalent to ContextForProjectRoot(projectRoot).WithProfile(profile).
func ContextForProjectAndProfile(projectRoot, profile string) *Context {
	return ContextForProjectRoot(projectRoot).WithProfile(profile)
}

// ContextFromInner wraps a loaded *context.Context (e.g. from ContextManager.LoadContext) with
// the default table output format. Returns nil if inner is nil.
func ContextFromInner(inner *context.Context) *Context {
	if inner == nil {
		return nil
	}
	return &Context{
		Context: inner,
		Format:  FormatTable,
	}
}
