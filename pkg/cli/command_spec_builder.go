package cli

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	specFlagTypeString      = "string"
	specFlagTypeBool        = "bool"
	specFlagTypeInt         = "int"
	specFlagTypeStringArray = "string_array"
)

var specFlagTypeToRuntimeType = map[string]FlagType{
	specFlagTypeString:      FlagTypeString,
	specFlagTypeBool:        FlagTypeBool,
	specFlagTypeInt:         FlagTypeInt,
	specFlagTypeStringArray: FlagTypeStringArray,
}

// ProcessorProvider provides access to processor/context for loading external resources
// This aligns with the coordinator pattern for context-constrained resource loading
// The internal/cli.Processor implements this interface via ProjectRoot() and OperationContext()
type ProcessorProvider interface {
	ProjectRoot() string
	OperationContext() context.Context
}

// CommandSpecBuilder builds a cobra.Command from a CommandSpec
// This bridges the declarative spec to the programmatic builder
type CommandSpecBuilder struct {
	spec         *CommandSpec
	runERegistry map[string]func(*cobra.Command, []string) error // Registry for RunE functions
	processor    ProcessorProvider                               // Optional processor for context-constrained loading
	specsDir     string                                          // Base directory for resolving spec references
}

// NewCommandSpecBuilder creates a new command spec builder
func NewCommandSpecBuilder(spec *CommandSpec) *CommandSpecBuilder {
	return &CommandSpecBuilder{
		spec:         spec,
		runERegistry: make(map[string]func(*cobra.Command, []string) error),
	}
}

// WithProcessor sets the processor provider for context-constrained resource loading
// This enables loading external spec references with proper project root resolution
func (b *CommandSpecBuilder) WithProcessor(processor ProcessorProvider) *CommandSpecBuilder {
	b.processor = processor
	return b
}

// WithSpecsDir sets the base directory for resolving spec references
// If not set, will use project root from processor if available
func (b *CommandSpecBuilder) WithSpecsDir(specsDir string) *CommandSpecBuilder {
	b.specsDir = specsDir
	return b
}

// RegisterRunE registers a RunE function by name
// This allows specs to reference RunE functions by name
func (b *CommandSpecBuilder) RegisterRunE(name string, runE func(*cobra.Command, []string) error) *CommandSpecBuilder {
	b.runERegistry[name] = runE
	return b
}

// Build builds a cobra.Command from the spec
func (b *CommandSpecBuilder) Build() (*cobra.Command, error) {
	if err := b.spec.Validate(); err != nil {
		return nil, errfmt.Newf("invalid command spec").Wrap(err)
	}

	builder := NewCommandBuilder(b.spec.Name)

	// Set short description
	if b.spec.Short != emptyValue {
		builder.WithShort(b.spec.Short)
	}

	// Long text: when Help is set, DynamicHelpBuilder owns description + examples (do not also set cmd.Long).
	if b.spec.Help == nil && b.spec.Description != emptyValue {
		builder.WithLong(b.spec.Description)
	}

	// Configure arguments
	if b.spec.Args != nil {
		args, err := b.buildArgs(b.spec.Args)
		if err != nil {
			return nil, errfmt.Newf("invalid args spec").Wrap(err)
		}
		builder.WithArgs(args)
	}

	// Build help builder if help spec is provided
	if b.spec.Help != nil {
		helpBuilder := b.buildHelpBuilder()
		builder.WithHelpBuilder(helpBuilder)
	}

	// Add flags
	for _, flagSpec := range b.spec.Flags {
		builder.AddFlag(b.buildFlag(flagSpec))
	}

	// Add subcommands
	for _, subcmdSpec := range b.spec.Subcommands {
		subcmd, err := b.buildSubcommand(subcmdSpec)
		if err != nil {
			return nil, errfmt.Errorf("failed to build subcommand %s: %w", subcmdSpec.Name, err)
		}
		builder.AddSubcommand(subcmd)
	}

	// Set RunE if specified
	if b.spec.RunE != emptyValue {
		runE, ok := b.runERegistry[b.spec.RunE]
		if !ok {
			return nil, errfmt.Errorf("RunE function '%s' not registered", b.spec.RunE)
		}
		builder.WithRunE(runE)
	}

	// Configure common flags
	if !b.spec.CommonFlags {
		builder.WithCommonFlags(false, nil)
	}

	// Configure entitlements
	if b.spec.EntitlementBundle != emptyValue {
		builder.WithEntitlementBundle(b.spec.EntitlementBundle)
	}

	cmd := builder.Build()
	applyOrchestrationFromSpec(cmd, b.spec.RequiresStorage, b.spec.RequiresSession, b.spec.RequiresSchedulerCheck)
	return cmd, nil
}

// buildArgs builds cobra.PositionalArgs from ArgsSpec
func (b *CommandSpecBuilder) buildArgs(spec *ArgsSpec) (cobra.PositionalArgs, error) {
	switch spec.Type {
	case "exact":
		if spec.Count == nil {
			return nil, errfmt.Errorf("exact args type requires count")
		}
		return cobra.ExactArgs(*spec.Count), nil
	case "minimum":
		if spec.Min == nil {
			return nil, errfmt.Errorf("minimum args type requires min")
		}
		return cobra.MinimumNArgs(*spec.Min), nil
	case "maximum":
		if spec.Max == nil {
			return nil, errfmt.Errorf("maximum args type requires max")
		}
		return cobra.MaximumNArgs(*spec.Max), nil
	case "range":
		if spec.Min == nil || spec.Max == nil {
			return nil, errfmt.Errorf("range args type requires both min and max")
		}
		return cobra.RangeArgs(*spec.Min, *spec.Max), nil
	case "no_args":
		return cobra.NoArgs, nil
	default:
		return nil, errfmt.Errorf("unknown args type: %s", spec.Type)
	}
}

// buildFlag builds FlagConfig from FlagSpec
func (b *CommandSpecBuilder) buildFlag(spec FlagSpec) FlagConfig {
	return FlagConfig{
		Name:        spec.Name,
		Shorthand:   spec.Shorthand,
		Type:        flagTypeFromSpecType(spec.Type),
		Default:     spec.Default,
		Description: spec.Description,
		Required:    spec.Required,
		// ValidateFunc would need to be resolved from registry if needed
	}
}

func flagTypeFromSpecType(specType string) FlagType {
	if flagType, ok := specFlagTypeToRuntimeType[specType]; ok {
		return flagType
	}
	return FlagTypeString // Default to string
}

// buildHelpBuilder builds HelpBuilder from HelpSpec
func (b *CommandSpecBuilder) buildHelpBuilder() *HelpBuilder {
	helpSpec := b.spec.Help
	short := helpSpec.Short
	if short == emptyValue {
		short = b.spec.Short
	}

	description := ""
	if len(helpSpec.Description) > 0 {
		description = helpSpec.Description[0]
		for i := 1; i < len(helpSpec.Description); i++ {
			description += "\n" + helpSpec.Description[i]
		}
	} else if b.spec.Description != emptyValue {
		description = b.spec.Description
	}

	helpBuilder := DynamicHelpBuilder(short, description)

	// Add examples
	for _, example := range helpSpec.Examples {
		helpBuilder.AddExample(example.Comment, example.Command)
	}

	// Exclude flags
	for _, flagName := range helpSpec.ExcludeFlags {
		helpBuilder.ExcludeFlags(flagName)
	}

	return helpBuilder
}

// buildSubcommand builds a subcommand from SubcommandSpec
func (b *CommandSpecBuilder) buildSubcommand(spec SubcommandSpec) (*cobra.Command, error) {
	if spec.Spec != nil {
		// Inline spec
		subBuilder := NewCommandSpecBuilder(spec.Spec)
		// Copy runE registry and processor
		for k, v := range b.runERegistry {
			subBuilder.RegisterRunE(k, v)
		}
		if b.processor != nil {
			subBuilder.WithProcessor(b.processor)
		}
		if b.specsDir != emptyValue {
			subBuilder.WithSpecsDir(b.specsDir)
		}
		return subBuilder.Build()
	} else if spec.SpecRef != emptyValue {
		// External spec reference - load from file using coordinator pattern
		// Resolve path relative to project root (from processor) or specsDir
		specPath, err := b.resolveSpecPath(spec.SpecRef)
		if err != nil {
			return nil, errfmt.Errorf("failed to resolve spec path for %s: %w", spec.SpecRef, err)
		}

		// Load spec from file
		loadedSpec, err := b.loadSpecFromFile(specPath)
		if err != nil {
			return nil, errfmt.Errorf("failed to load spec from %s: %w", specPath, err)
		}

		// Build subcommand from loaded spec
		subBuilder := NewCommandSpecBuilder(loadedSpec)
		// Copy runE registry and processor
		for k, v := range b.runERegistry {
			subBuilder.RegisterRunE(k, v)
		}
		if b.processor != nil {
			subBuilder.WithProcessor(b.processor)
		}
		if b.specsDir != emptyValue {
			subBuilder.WithSpecsDir(b.specsDir)
		}
		return subBuilder.Build()
	} else {
		// Simple subcommand with just a name
		return &cobra.Command{Use: spec.Name}, nil
	}
}

// resolveSpecPath resolves a spec reference to an absolute file path
// Uses coordinator pattern: project root from processor > specsDir > current directory
func (b *CommandSpecBuilder) resolveSpecPath(specRef string) (string, error) {
	// If already absolute, return as-is
	if filepath.IsAbs(specRef) {
		return specRef, nil
	}

	// Try to resolve relative to specsDir or project root (coordinator pattern)
	var baseDir string
	if b.specsDir != emptyValue {
		baseDir = b.specsDir
	} else if b.processor != nil {
		projectRoot := b.processor.ProjectRoot()
		if projectRoot != emptyValue {
			// Use standard command specs directory structure
			baseDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.CLISpecsDir)
		}
	}

	if baseDir == emptyValue {
		// Fallback to current directory
		cwd, err := fileutil.Getwd()
		if err != nil {
			return "", errfmt.Newf("failed to get current directory").Wrap(err)
		}
		baseDir = cwd
	}

	// Resolve path (coordinator pattern: respect context constraints)
	resolved := filepath.Join(baseDir, specRef)

	// Ensure it's absolute
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return "", errfmt.Newf("failed to resolve absolute path").Wrap(err)
	}

	// Validate path is within project root (security constraint - coordinator pattern)
	if b.processor != nil {
		projectRoot := b.processor.ProjectRoot()
		if projectRoot != emptyValue {
			// Ensure resolved path is within project root (coordinator pattern constraint)
			relPath, err := filepath.Rel(projectRoot, absPath)
			if err != nil {
				return "", errfmt.Newf("spec path outside project root").Wrap(err)
			}
			// Check for path traversal attempts
			if filepath.IsAbs(relPath) || relPath == ".." || len(relPath) >= 3 && relPath[:3] == "../" {
				return "", errfmt.Errorf("spec path traversal detected: %s", relPath)
			}
		}
	}

	return absPath, nil
}

// loadSpecFromFile loads a CommandSpec from a YAML file
func (b *CommandSpecBuilder) loadSpecFromFile(filePath string) (*CommandSpec, error) {
	// Check if file exists
	if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
		return nil, errfmt.Errorf("spec file does not exist: %s", filePath)
	}

	// Read file
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read spec file").Wrap(err)
	}

	// Parse YAML
	var spec CommandSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Newf("failed to parse spec file").Wrap(err)
	}

	// Validate spec
	if err := spec.Validate(); err != nil {
		return nil, errfmt.Errorf("invalid spec in file %s: %w", filePath, err)
	}

	return &spec, nil
}

// CRUDCommandSpecBuilder builds a cobra.Command from a CRUDCommandSpec
type CRUDCommandSpecBuilder struct {
	spec         *CRUDCommandSpec
	runERegistry map[string]func(*cobra.Command, []string) error
	processor    ProcessorProvider
	specsDir     string
}

// NewCRUDCommandSpecBuilder creates a new CRUD command spec builder
func NewCRUDCommandSpecBuilder(spec *CRUDCommandSpec) *CRUDCommandSpecBuilder {
	return &CRUDCommandSpecBuilder{
		spec:         spec,
		runERegistry: make(map[string]func(*cobra.Command, []string) error),
	}
}

// WithProcessor sets the processor provider for context-constrained resource loading
func (b *CRUDCommandSpecBuilder) WithProcessor(processor ProcessorProvider) *CRUDCommandSpecBuilder {
	b.processor = processor
	return b
}

// WithSpecsDir sets the base directory for resolving spec references
func (b *CRUDCommandSpecBuilder) WithSpecsDir(specsDir string) *CRUDCommandSpecBuilder {
	b.specsDir = specsDir
	return b
}

// RegisterRunE registers a RunE function by name
func (b *CRUDCommandSpecBuilder) RegisterRunE(name string, runE func(*cobra.Command, []string) error) *CRUDCommandSpecBuilder {
	b.runERegistry[name] = runE
	return b
}

// Build builds a cobra.Command from the CRUD spec
func (b *CRUDCommandSpecBuilder) Build() (*cobra.Command, error) {
	if err := b.spec.Validate(); err != nil {
		return nil, errfmt.Newf("invalid CRUD command spec").Wrap(err)
	}

	// Build base command using CRUDCommandBuilder
	crudBuilder := NewCRUDCommandBuilder(b.spec.OperationType, b.spec.Name)

	// Set short description
	if b.spec.Short != emptyValue {
		crudBuilder.WithShort(b.spec.Short)
	}

	// Build help
	if b.spec.Help != nil {
		examples := make([]string, 0)
		for _, example := range b.spec.Help.Examples {
			examples = append(examples, example.Comment, example.Command)
		}
		description := ""
		if len(b.spec.Help.Description) > 0 {
			description = b.spec.Help.Description[0]
			for i := 1; i < len(b.spec.Help.Description); i++ {
				description += "\n" + b.spec.Help.Description[i]
			}
		} else {
			description = b.spec.Description
		}
		crudBuilder.WithCRUDHelp(b.spec.Short, description, examples...)
	}

	// Configure arguments
	if b.spec.Args != nil {
		args, err := b.buildArgs(b.spec.Args)
		if err != nil {
			return nil, errfmt.Newf("invalid args spec").Wrap(err)
		}
		crudBuilder.WithArgs(args)
	}

	// Add CRUD-specific flags (only if not already in custom flags)
	customFlagNames := make(map[string]bool)
	for _, flagSpec := range b.spec.Flags {
		customFlagNames[flagSpec.Name] = true
	}

	if b.spec.DataInput {
		crudBuilder.WithDataInputFlags()
	}
	if b.spec.UpdateFlags {
		crudBuilder.WithUpdateFlags()
	}
	if b.spec.DryRun && !customFlagNames[crudFlagDryRun] {
		crudBuilder.WithDryRunFlag()
	}
	if b.spec.Cascade && !customFlagNames[crudFlagCascade] {
		crudBuilder.WithCascadeFlag()
	}
	if b.spec.UnlinkReferences && !customFlagNames[crudFlagUnlinkReferences] {
		crudBuilder.WithUnlinkReferencesFlag()
	}
	if b.spec.QueryFlags {
		crudBuilder.WithQueryFlags()
	}

	// Add custom flags (may include cascade/dry-run if explicitly specified)
	for _, flagSpec := range b.spec.Flags {
		crudBuilder.AddFlag(b.buildFlag(flagSpec))
	}

	// Set RunE if specified
	if b.spec.RunE != emptyValue {
		runE, ok := b.runERegistry[b.spec.RunE]
		if !ok {
			return nil, errfmt.Errorf("RunE function '%s' not registered", b.spec.RunE)
		}
		crudBuilder.WithRunE(runE)
	}

	// Configure common flags
	if !b.spec.CommonFlags {
		crudBuilder.WithCommonFlags(false, nil)
	}

	// Configure entitlements
	if b.spec.EntitlementBundle != emptyValue {
		crudBuilder.WithEntitlementBundle(b.spec.EntitlementBundle)
	}

	cmd := crudBuilder.Build()
	applyOrchestrationFromSpec(cmd, b.spec.RequiresStorage, b.spec.RequiresSession, b.spec.RequiresSchedulerCheck)
	return cmd, nil
}

// buildArgs is a helper shared with CommandSpecBuilder
func (b *CRUDCommandSpecBuilder) buildArgs(spec *ArgsSpec) (cobra.PositionalArgs, error) {
	builder := NewCommandSpecBuilder(&CommandSpec{})
	return builder.buildArgs(spec)
}

// buildFlag is a helper shared with CommandSpecBuilder
func (b *CRUDCommandSpecBuilder) buildFlag(spec FlagSpec) FlagConfig {
	builder := NewCommandSpecBuilder(&CommandSpec{})
	return builder.buildFlag(spec)
}

// orchestration annotation keys (must match internal/cli/command_requirements.go)
const (
	annRequiresStorage        = "zqk.requires_storage"
	annRequiresSession        = "zqk.requires_session"
	annRequiresSchedulerCheck = "zqk.requires_scheduler_check"
	annotationBoolFalse       = "false"
)

// applyOrchestrationFromSpec sets cobra annotations from CommandSpec orchestration fields
// so root's GetCommandRequirements sees them. Only sets when value is false (opt-out).
func applyOrchestrationFromSpec(cmd *cobra.Command, requiresStorage, requiresSession, requiresSchedulerCheck *bool) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	if requiresStorage != nil && !*requiresStorage {
		cmd.Annotations[annRequiresStorage] = annotationBoolFalse
	}
	if requiresSession != nil && !*requiresSession {
		cmd.Annotations[annRequiresSession] = annotationBoolFalse
	}
	if requiresSchedulerCheck != nil && !*requiresSchedulerCheck {
		cmd.Annotations[annRequiresSchedulerCheck] = annotationBoolFalse
	}
}
