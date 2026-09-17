package context

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const (
	contextLayerNameSystem  = "system"
	contextLayerNameUser    = "user"
	contextLayerNameProject = "project"
	contextLayerNameCommand = "command"
	contextDefaultFormat    = "table"
)

// ContextBuilderPattern provides a fluent API for building contexts
// This is a convenience wrapper around ContextBuilder that makes common patterns easier
type ContextBuilderPattern struct {
	builder *ContextBuilder
}

// NewContextBuilderPattern creates a new context builder pattern helper
func NewContextBuilderPattern() *ContextBuilderPattern {
	return &ContextBuilderPattern{
		builder: NewContextBuilder(ModeSequential),
	}
}

// WithSystemDefaults adds system default context
func (p *ContextBuilderPattern) WithSystemDefaults(defaults map[string]any) *ContextBuilderPattern {
	if defaults == nil {
		defaults = getDefaultSystemDefaults()
	}
	systemCtx := &Context{
		layers: make(map[ContextLayer]map[string]any),
	}
	systemCtx.layers[LayerSystem] = defaults
	applyDefaultsToContext(systemCtx, defaults)
	p.builder.AddContext(systemCtx, contextLayerNameSystem)
	return p
}

// WithUserConfig adds user-level configuration context
func (p *ContextBuilderPattern) WithUserConfig(config map[string]any) *ContextBuilderPattern {
	if len(config) == 0 {
		return p
	}
	userCtx := &Context{
		layers: make(map[ContextLayer]map[string]any),
	}
	userCtx.layers[LayerUser] = config
	applyDefaultsToContext(userCtx, config)
	p.builder.AddContext(userCtx, contextLayerNameUser)
	return p
}

// WithProjectConfig adds project-level configuration context
func (p *ContextBuilderPattern) WithProjectConfig(projectRoot string, config map[string]any) *ContextBuilderPattern {
	if len(config) == 0 {
		return p
	}
	projectCtx := &Context{
		layers: make(map[ContextLayer]map[string]any),
	}
	projectCtx.layers[LayerProject] = config
	projectCtx.ProjectRoot = projectRoot
	applyDefaultsToContext(projectCtx, config)
	p.builder.AddContext(projectCtx, contextLayerNameProject)
	return p
}

// WithCommandFlags adds command-line flag context (highest precedence)
func (p *ContextBuilderPattern) WithCommandFlags(flags map[string]any) *ContextBuilderPattern {
	if len(flags) == 0 {
		return p
	}
	commandCtx := &Context{
		layers: make(map[ContextLayer]map[string]any),
	}
	commandCtx.layers[LayerCommand] = flags
	applyDefaultsToContext(commandCtx, flags)
	p.builder.AddContext(commandCtx, contextLayerNameCommand)
	return p
}

// WithOverride adds a single override value (treated as command layer)
func (p *ContextBuilderPattern) WithOverride(key string, value any) *ContextBuilderPattern {
	overrides := map[string]any{key: value}
	return p.WithCommandFlags(overrides)
}

// WithOverrides adds multiple override values (treated as command layer)
func (p *ContextBuilderPattern) WithOverrides(overrides map[string]any) *ContextBuilderPattern {
	return p.WithCommandFlags(overrides)
}

// Build creates the final merged context
func (p *ContextBuilderPattern) Build() (*Context, error) {
	return p.builder.Build()
}

// MustBuild creates the final merged context, panicking on error
// Use this when you're certain the context will build successfully
func (p *ContextBuilderPattern) MustBuild() *Context {
	ctx, err := p.builder.Build()
	if err != nil {
		// TRACK: [Initialization failure]
		panic(fmt.Sprintf("failed to build context: %v", err))
	}
	return ctx
}

// Helper functions

func getDefaultSystemDefaults() map[string]any {
	return map[string]any{
		contextKeyFormat:       contextDefaultFormat,
		contextKeyVerbose:      false,
		contextKeyQuiet:        false,
		contextKeyProfile:      "",
		contextKeyProjectRoot:  "",
		contextKeyPriorityPlan: "",
		contextKeyWorkstream:   "",
		contextKeyMilestone:    "",
	}
}

func applyDefaultsToContext(ctx *Context, config map[string]any) {
	// Use the same logic as createContextFromMap in context.go
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
	// Apply profile
	if profile, ok := config[contextKeyProfile].(string); ok && profile != emptyValue {
		ctx.Profile = profile
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
	// Apply storage context
	if storageCtx, ok := config[contextKeyStorage].(map[string]any); ok {
		switch maxPageSize := storageCtx[contextKeyMaxPageSize].(type) {
		case int:
			ctx.StorageMaxPageSize = maxPageSize
		case float64:
			ctx.StorageMaxPageSize = int(maxPageSize)
		}
		switch defaultPageSize := storageCtx[contextKeyDefaultPageSize].(type) {
		case int:
			ctx.StorageDefaultPageSize = defaultPageSize
		case float64:
			ctx.StorageDefaultPageSize = int(defaultPageSize)
		}
		if enableGrouping, ok := storageCtx[contextKeyEnableGrouping].(bool); ok {
			ctx.StorageEnableGrouping = enableGrouping
		}
		switch maxGroupSize := storageCtx[contextKeyMaxGroupSize].(type) {
		case int:
			ctx.StorageMaxGroupSize = maxGroupSize
		case float64:
			ctx.StorageMaxGroupSize = int(maxGroupSize)
		}
	}
	if v := resolveErrorLogOutput(config); v != emptyValue {
		ctx.ErrorLogOutput = v
	}
}

func resolveErrorLogOutput(config map[string]any) string {
	if m, ok := config["logging"].(map[string]any); ok {
		if v := errorLogOutputFromMap(m); v != emptyValue {
			return v
		}
	}
	return errorLogOutputFromMap(config)
}

func errorLogOutputFromMap(m map[string]any) string {
	s, _ := m[contextKeyErrorLogOutput].(string)
	if s == "separate" || s == "combined" {
		return s
	}
	return ""
}

// BuildContextFromLayers is a convenience function that builds a context from all layers
// This is the standard pattern used by LoadContext
func BuildContextFromLayers(systemDefaults, userConfig, projectConfig, commandFlags map[string]any, projectRoot string) (*Context, error) {
	pattern := NewContextBuilderPattern()
	pattern.WithSystemDefaults(systemDefaults)
	pattern.WithUserConfig(userConfig)
	if projectRoot != emptyValue {
		pattern.WithProjectConfig(projectRoot, projectConfig)
	}
	pattern.WithCommandFlags(commandFlags)
	return pattern.Build()
}

// BuildContextWithOverrides builds a base context and applies overrides
// This is useful for deriving contexts from an existing one
// Note: This is a convenience wrapper around Context.Derive()
// Prefer using base.Derive() directly for better performance
func BuildContextWithOverrides(base *Context, overrides map[string]any) (*Context, error) {
	if base == nil {
		return nil, errfmt.Errorf("base context is required")
	}

	// Use the existing Derive method (returns *Context, not error)
	return base.Derive(overrides), nil
}

// BuildContextForCommand builds a context specifically for a command execution
// This combines system, user, project, and command flags in the standard precedence order
func BuildContextForCommand(systemDefaults, userConfig, projectConfig, commandFlags map[string]any, projectRoot string) (*Context, error) {
	return BuildContextFromLayers(systemDefaults, userConfig, projectConfig, commandFlags, projectRoot)
}
