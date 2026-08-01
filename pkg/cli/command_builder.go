package cli

import (
	"context"

	"github.com/spf13/cobra"
)

// CheckEntitlementFunc is an injected function to check entitlements.
// It avoids coupling pkg/cli with pkg/entitlements directly.
var CheckEntitlementFunc func(ctx context.Context, bundle string) error

var defaultHelpExcludedCommonFlags = []string{
	"format",
	"output",
	"verbose",
	"quiet",
	"timeout",
	"columns",
}

// DefaultCommonExcludedFlags returns a copy of the canonical common-flag exclusion list.
func DefaultCommonExcludedFlags() []string {
	return append([]string(nil), defaultHelpExcludedCommonFlags...)
}

// CommonExcludedFlagsWithout returns canonical common-flag exclusions minus one flag name.
func CommonExcludedFlagsWithout(flagName string) []string {
	excludes := make([]string, 0, len(defaultHelpExcludedCommonFlags))
	for _, name := range defaultHelpExcludedCommonFlags {
		if name != flagName {
			excludes = append(excludes, name)
		}
	}
	return excludes
}

const (
	crudFlagFile             = "file"
	crudFlagData             = "data"
	crudFlagField            = "field"
	crudFlagAutoStatus       = "auto-status"
	crudFlagDryRun           = "dry-run"
	crudFlagCascade          = "cascade"
	crudFlagUnlinkReferences = "unlink-references"
	crudFlagFilter           = "filter"
	crudFlagSortBy           = "sort-by"
	crudFlagSortAsc          = "sort-asc"
	crudFlagOffset           = "offset"
	crudFlagLimit            = "limit"
	crudFlagCount            = "count"
	crudFlagFields           = "fields"
	crudFlagGroupBy          = "group-by"
	crudFlagGroupLimit       = "group-limit"

	crudHelpFileObjectData = "Path to YAML file containing object data"
	crudHelpInlineObject   = "Inline YAML data for the object"
	crudHelpFileUpdateData = "Path to YAML file containing update data"
	crudHelpInlineUpdate   = "Inline YAML data for updates"
	crudHelpUpdateField    = "Update a field (field=value or field:value). Parses as JSON when valid, else literal string; use JSON quotes for colons (e.g. note=\"a: b\"). Repeatable."
	crudHelpAutoStatus     = "Advance status to the next lifecycle-valid status for this object kind. Cannot be combined with explicit status field."
	crudHelpDryRun         = "Show what would be done without actually doing it"
	crudHelpFilter         = "Filter by field (format: field=value or field:value, can be used multiple times)"
	crudHelpSortBy         = "Field to sort by"
	crudHelpSortAsc        = "Sort ascending (default: true)"
	crudHelpOffset         = "Pagination offset"
	crudHelpLimit          = "Pagination limit"
	crudHelpCount          = "Only return the count of matching objects (no object details)"
	crudHelpFields         = "Keys to include per object (repeat or comma-separated); omit for full rows. Sort/group columns merge automatically."
	crudHelpGroupBy        = "Field to group results by"
	crudHelpGroupLimit     = "Maximum items per group when using --group-by (0 = no limit)"
)

// CommandBuilder provides a fluent builder pattern for creating consistent CLI commands
// This standardizes command structure across all commands and reduces boilerplate
type CommandBuilder struct {
	use         string
	short       string
	long        string
	args        cobra.PositionalArgs
	runE        func(*cobra.Command, []string) error // cobra.CommandRunE
	helpBuilder *HelpBuilder                         // Note: HelpBuilder is from help_builder.go

	// Flag configuration
	commonFlags            bool
	commonFlagsFunc        func(*cobra.Command)           // Function to add common flags (avoids import cycle)
	commonFlagsExclude     []string                       // When set, add common flags excluding these (used with commonFlagsExcludeFunc)
	commonFlagsExcludeFunc func(*cobra.Command, []string) // Add common flags with exclusions (caller provides e.g. cli.AddCommonFlagsExcluding)
	customFlags            []FlagConfig
	requiredFlags          []string

	// Subcommands
	subcommands []*cobra.Command

	// Help customization
	helpFunc func(*cobra.Command, []string) // cobra.HelpFunc

	// Entitlements
	entitlementBundle string
}

// FlagConfig represents a flag to be added to the command
type FlagConfig struct {
	Name         string
	Shorthand    string
	Type         FlagType
	Default      any
	Description  string
	Required     bool
	ValidateFunc func(string) error
}

// FlagType represents the type of flag
type FlagType int

const (
	FlagTypeString FlagType = iota
	FlagTypeBool
	FlagTypeInt
	FlagTypeStringArray
	FlagTypeStringSlice
	FlagTypeIntSlice
	FlagTypeStringToString
	FlagTypeFloat
)

// NewCommandBuilder creates a new command builder
func NewCommandBuilder(use string) *CommandBuilder {
	return &CommandBuilder{
		use:           use,
		commonFlags:   true, // Default to including common flags
		customFlags:   make([]FlagConfig, 0),
		requiredFlags: make([]string, 0),
		subcommands:   make([]*cobra.Command, 0),
	}
}

// WithShort sets the short description
func (b *CommandBuilder) WithShort(short string) *CommandBuilder {
	b.short = short
	return b
}

// WithLong sets the long description (overrides help builder if set)
func (b *CommandBuilder) WithLong(long string) *CommandBuilder {
	b.long = long
	return b
}

// WithHelpBuilder sets the help builder for dynamic help generation
func (b *CommandBuilder) WithHelpBuilder(helpBuilder *HelpBuilder) *CommandBuilder {
	b.helpBuilder = helpBuilder
	return b
}

// WithArgs sets the argument validation function
func (b *CommandBuilder) WithArgs(args cobra.PositionalArgs) *CommandBuilder {
	b.args = args
	return b
}

// WithRunE sets the command execution function
func (b *CommandBuilder) WithRunE(runE func(*cobra.Command, []string) error) *CommandBuilder {
	b.runE = runE
	return b
}

// WithCommonFlags enables/disables common flags (format, output, verbose, quiet, timeout)
// The addCommonFlagsFunc parameter is a function that adds common flags to the command
// This avoids import cycles (typically: func(cmd *cobra.Command) { cli.AddCommonFlags(cmd) })
func (b *CommandBuilder) WithCommonFlags(enabled bool, addCommonFlagsFunc func(*cobra.Command)) *CommandBuilder {
	b.commonFlags = enabled
	b.commonFlagsFunc = addCommonFlagsFunc
	return b
}

// WithCommonFlagsDefault enables common flags with a default function (caller must provide)
// This is a convenience method that requires the caller to pass the function
func (b *CommandBuilder) WithCommonFlagsDefault(addCommonFlagsFunc func(*cobra.Command)) *CommandBuilder {
	b.commonFlags = true
	b.commonFlagsFunc = addCommonFlagsFunc
	b.commonFlagsExclude = nil
	b.commonFlagsExcludeFunc = nil
	return b
}

// WithCommonFlagsExcluding enables common flags but adds only those not in exclude (so they do not appear on the command or in help).
// Caller must pass a function that adds common flags with exclusions (e.g. cli.AddCommonFlagsExcluding).
func (b *CommandBuilder) WithCommonFlagsExcluding(addCommonFlagsExcludingFunc func(*cobra.Command, []string), exclude []string) *CommandBuilder {
	b.commonFlags = true
	b.commonFlagsFunc = nil
	b.commonFlagsExclude = exclude
	b.commonFlagsExcludeFunc = addCommonFlagsExcludingFunc
	return b
}

// WithCommonFlagsDefaultExcluding applies the canonical common-flag exclusion set.
func (b *CommandBuilder) WithCommonFlagsDefaultExcluding(addCommonFlagsExcludingFunc func(*cobra.Command, []string)) *CommandBuilder {
	return b.WithCommonFlagsExcluding(addCommonFlagsExcludingFunc, DefaultCommonExcludedFlags())
}

// WithCommonFlagsDefaultExcludingWithout applies canonical common exclusions minus one flag.
func (b *CommandBuilder) WithCommonFlagsDefaultExcludingWithout(
	addCommonFlagsExcludingFunc func(*cobra.Command, []string),
	flagName string,
) *CommandBuilder {
	return b.WithCommonFlagsExcluding(addCommonFlagsExcludingFunc, CommonExcludedFlagsWithout(flagName))
}

// AddFlag adds a custom flag to the command
func (b *CommandBuilder) AddFlag(config FlagConfig) *CommandBuilder {
	b.customFlags = append(b.customFlags, config)
	if config.Required {
		b.requiredFlags = append(b.requiredFlags, config.Name)
	}
	return b
}

// AddStringFlag adds a string flag
func (b *CommandBuilder) AddStringFlag(name, shorthand, defaultValue, description string) *CommandBuilder {
	return b.AddFlag(FlagConfig{
		Name:        name,
		Shorthand:   shorthand,
		Type:        FlagTypeString,
		Default:     defaultValue,
		Description: description,
	})
}

// AddBoolFlag adds a boolean flag
func (b *CommandBuilder) AddBoolFlag(name, shorthand string, defaultValue bool, description string) *CommandBuilder {
	return b.AddFlag(FlagConfig{
		Name:        name,
		Shorthand:   shorthand,
		Type:        FlagTypeBool,
		Default:     defaultValue,
		Description: description,
	})
}

// AddIntFlag adds an integer flag
func (b *CommandBuilder) AddIntFlag(name, shorthand string, defaultValue int, description string) *CommandBuilder {
	return b.AddFlag(FlagConfig{
		Name:        name,
		Shorthand:   shorthand,
		Type:        FlagTypeInt,
		Default:     defaultValue,
		Description: description,
	})
}

// AddStringArrayFlag adds a string array flag
func (b *CommandBuilder) AddStringArrayFlag(name, shorthand, description string) *CommandBuilder {
	return b.AddFlag(FlagConfig{
		Name:        name,
		Shorthand:   shorthand,
		Type:        FlagTypeStringArray,
		Description: description,
	})
}

// AddStringSliceFlag adds a string slice flag (comma-separated and repeatable; see cobra StringSlice).
func (b *CommandBuilder) AddStringSliceFlag(name, shorthand, description string) *CommandBuilder {
	return b.AddFlag(FlagConfig{
		Name:        name,
		Shorthand:   shorthand,
		Type:        FlagTypeStringSlice,
		Description: description,
	})
}

// AddStringToStringFlag adds a string-to-string map flag
func (b *CommandBuilder) AddStringToStringFlag(name, shorthand, description string) *CommandBuilder {
	return b.AddFlag(FlagConfig{
		Name:        name,
		Shorthand:   shorthand,
		Type:        FlagTypeStringToString,
		Description: description,
	})
}

// AddFloatFlag adds a float flag
func (b *CommandBuilder) AddFloatFlag(name, shorthand string, defaultValue float64, description string) *CommandBuilder {
	return b.AddFlag(FlagConfig{
		Name:        name,
		Shorthand:   shorthand,
		Type:        FlagTypeFloat,
		Default:     defaultValue,
		Description: description,
	})
}

// AddSubcommand adds a subcommand to the command
func (b *CommandBuilder) AddSubcommand(subcmd *cobra.Command) *CommandBuilder {
	b.subcommands = append(b.subcommands, subcmd)
	return b
}

// WithHelpFunc sets a custom help function
func (b *CommandBuilder) WithHelpFunc(helpFunc func(*cobra.Command, []string)) *CommandBuilder {
	b.helpFunc = helpFunc
	return b
}

// WithEntitlementBundle sets the required entitlement bundle for this command and its subcommands
func (b *CommandBuilder) WithEntitlementBundle(bundle string) *CommandBuilder {
	b.entitlementBundle = bundle
	return b
}

// WithQueryFlags adds standard query flags (filter, sort, pagination)
func (b *CommandBuilder) WithQueryFlags() *CommandBuilder {
	b.AddStringArrayFlag(crudFlagFilter, "", crudHelpFilter)
	b.AddStringFlag(crudFlagSortBy, "", "", crudHelpSortBy)
	b.AddBoolFlag(crudFlagSortAsc, "", true, crudHelpSortAsc)
	b.AddIntFlag(crudFlagOffset, "", 0, crudHelpOffset)
	b.AddIntFlag(crudFlagLimit, "", 0, crudHelpLimit)
	b.AddBoolFlag(crudFlagCount, "", false, crudHelpCount)
	b.AddStringArrayFlag(crudFlagFields, "", crudHelpFields)
	b.AddStringFlag(crudFlagGroupBy, "", "", crudHelpGroupBy)
	b.AddIntFlag(crudFlagGroupLimit, "", 0, crudHelpGroupLimit)
	return b
}

// Build creates and returns the cobra.Command
func (b *CommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:          b.use,
		RunE:         b.runE,
		SilenceUsage: true,
	}

	// Set short description
	if b.short != emptyValue {
		cmd.Short = b.short
	}

	// Set long description (from help builder or explicit)
	if b.long != emptyValue {
		cmd.Long = b.long
	} else if b.helpBuilder != nil {
		// Apply help builder
		b.helpBuilder.ApplyToCommand(cmd)
	}

	// Set argument validation
	if b.args != nil {
		cmd.Args = b.args
	}

	// Add common flags (with optional exclusions so excluded flags are not on the command)
	if b.commonFlags {
		if b.commonFlagsExcludeFunc != nil && len(b.commonFlagsExclude) > 0 {
			b.commonFlagsExcludeFunc(cmd, b.commonFlagsExclude)
		} else if b.commonFlagsFunc != nil {
			b.commonFlagsFunc(cmd)
		}
	}

	// Add custom flags
	for _, flagConfig := range b.customFlags {
		switch flagConfig.Type {
		case FlagTypeString:
			defaultValue := ""
			if flagConfig.Default != nil {
				defaultValue = flagConfig.Default.(string)
			}
			if flagConfig.Shorthand != emptyValue {
				cmd.Flags().StringP(flagConfig.Name, flagConfig.Shorthand, defaultValue, flagConfig.Description)
			} else {
				cmd.Flags().String(flagConfig.Name, defaultValue, flagConfig.Description)
			}
		case FlagTypeBool:
			defaultValue := false
			if flagConfig.Default != nil {
				defaultValue = flagConfig.Default.(bool)
			}
			if flagConfig.Shorthand != emptyValue {
				cmd.Flags().BoolP(flagConfig.Name, flagConfig.Shorthand, defaultValue, flagConfig.Description)
			} else {
				cmd.Flags().Bool(flagConfig.Name, defaultValue, flagConfig.Description)
			}
		case FlagTypeInt:
			defaultValue := 0
			if flagConfig.Default != nil {
				defaultValue = flagConfig.Default.(int)
			}
			if flagConfig.Shorthand != emptyValue {
				cmd.Flags().IntP(flagConfig.Name, flagConfig.Shorthand, defaultValue, flagConfig.Description)
			} else {
				cmd.Flags().Int(flagConfig.Name, defaultValue, flagConfig.Description)
			}
		case FlagTypeStringArray:
			if flagConfig.Shorthand != emptyValue {
				cmd.Flags().StringArrayP(flagConfig.Name, flagConfig.Shorthand, []string{}, flagConfig.Description)
			} else {
				cmd.Flags().StringArray(flagConfig.Name, []string{}, flagConfig.Description)
			}
		case FlagTypeStringSlice:
			if flagConfig.Shorthand != emptyValue {
				cmd.Flags().StringSliceP(flagConfig.Name, flagConfig.Shorthand, []string{}, flagConfig.Description)
			} else {
				cmd.Flags().StringSlice(flagConfig.Name, []string{}, flagConfig.Description)
			}
		case FlagTypeStringToString:
			defaultValue := map[string]string{}
			if flagConfig.Default != nil {
				if m, ok := flagConfig.Default.(map[string]string); ok {
					defaultValue = m
				}
			}
			if flagConfig.Shorthand != emptyValue {
				cmd.Flags().StringToStringP(flagConfig.Name, flagConfig.Shorthand, defaultValue, flagConfig.Description)
			} else {
				cmd.Flags().StringToString(flagConfig.Name, defaultValue, flagConfig.Description)
			}
		case FlagTypeFloat:
			defaultValue := 0.0
			if flagConfig.Default != nil {
				if f, ok := flagConfig.Default.(float64); ok {
					defaultValue = f
				}
			}
			if flagConfig.Shorthand != emptyValue {
				cmd.Flags().Float64P(flagConfig.Name, flagConfig.Shorthand, defaultValue, flagConfig.Description)
			} else {
				cmd.Flags().Float64(flagConfig.Name, defaultValue, flagConfig.Description)
			}
		}

		// Mark as required if specified
		if flagConfig.Required {
			_ = cmd.MarkFlagRequired(flagConfig.Name)
		}
	}

	// Add subcommands
	for _, subcmd := range b.subcommands {
		cmd.AddCommand(subcmd)
	}

	// Set custom help function if provided
	if b.helpFunc != nil {
		cmd.SetHelpFunc(b.helpFunc)
	}

	// Apply entitlement checks
	if b.entitlementBundle != "" {
		cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
			if CheckEntitlementFunc != nil {
				if err := CheckEntitlementFunc(c.Context(), b.entitlementBundle); err != nil {
					return err
				}
			}
			return nil
		}
	}

	return cmd
}

// CRUDCommandBuilder provides specialized builders for common CRUD operations
type CRUDCommandBuilder struct {
	*CommandBuilder
	operationType string // "create", "read", "update", "delete", "list"
}

// NewCRUDCommandBuilder creates a builder for CRUD operations
func NewCRUDCommandBuilder(operationType, use string) *CRUDCommandBuilder {
	return &CRUDCommandBuilder{
		CommandBuilder: NewCommandBuilder(use),
		operationType:  operationType,
	}
}

// WithCRUDHelp sets up standard CRUD help text
func (b *CRUDCommandBuilder) WithCRUDHelp(short, description string, examples ...string) *CRUDCommandBuilder {
	helpBuilder := DynamicHelpBuilder(short, description)

	// Add examples
	for i := 0; i < len(examples); i += 2 {
		if i+1 < len(examples) {
			helpBuilder.AddExample(examples[i], examples[i+1])
		}
	}

	// Exclude common flags from auto-discovery
	helpBuilder.ExcludeFlags(defaultHelpExcludedCommonFlags...)

	b.WithHelpBuilder(helpBuilder)
	return b
}

// WithArgs sets the argument validation function (CRUDCommandBuilder version)
func (b *CRUDCommandBuilder) WithArgs(args cobra.PositionalArgs) *CRUDCommandBuilder {
	b.CommandBuilder.WithArgs(args)
	return b
}

// WithRunE sets the command execution function (CRUDCommandBuilder version)
func (b *CRUDCommandBuilder) WithRunE(runE func(*cobra.Command, []string) error) *CRUDCommandBuilder {
	b.CommandBuilder.WithRunE(runE)
	return b
}

// WithCommonFlagsDefault enables common flags with a default function (CRUDCommandBuilder version)
func (b *CRUDCommandBuilder) WithCommonFlagsDefault(addCommonFlagsFunc func(*cobra.Command)) *CRUDCommandBuilder {
	b.CommandBuilder.WithCommonFlagsDefault(addCommonFlagsFunc)
	return b
}

// WithCommonFlagsExcluding enables common flags with exclusions (CRUDCommandBuilder version)
func (b *CRUDCommandBuilder) WithCommonFlagsExcluding(addFunc func(*cobra.Command, []string), exclude []string) *CRUDCommandBuilder {
	b.CommandBuilder.WithCommonFlagsExcluding(addFunc, exclude)
	return b
}

// WithDataInputFlags adds standard data input flags (file, data, stdin)
func (b *CRUDCommandBuilder) WithDataInputFlags() *CRUDCommandBuilder {
	b.AddStringFlag(crudFlagFile, "", "", crudHelpFileObjectData)
	b.AddStringFlag(crudFlagData, "", "", crudHelpInlineObject)
	b.AddStringArrayFlag(crudFlagField, "", "Field to set in the format key=value (can be specified multiple times)")
	return b
}

// WithUpdateFlags adds standard update flags (file, data, field, auto-status)
func (b *CRUDCommandBuilder) WithUpdateFlags() *CRUDCommandBuilder {
	b.AddStringFlag(crudFlagFile, "", "", crudHelpFileUpdateData)
	b.AddStringFlag(crudFlagData, "", "", crudHelpInlineUpdate)
	b.AddStringArrayFlag(crudFlagField, "", crudHelpUpdateField)
	b.AddBoolFlag(crudFlagAutoStatus, "", false, crudHelpAutoStatus)
	return b
}

// WithDryRunFlag adds a dry-run flag
func (b *CRUDCommandBuilder) WithDryRunFlag() *CRUDCommandBuilder {
	b.AddBoolFlag(crudFlagDryRun, "", false, crudHelpDryRun)
	return b
}

// WithCascadeFlag adds a cascade flag (for delete operations)
func (b *CRUDCommandBuilder) WithCascadeFlag() *CRUDCommandBuilder {
	b.AddBoolFlag(crudFlagCascade, "", false, "Delete object and all objects that reference it")
	return b
}

// WithUnlinkReferencesFlag removes references to this object from dependents instead of deleting them or failing (delete operations).
func (b *CRUDCommandBuilder) WithUnlinkReferencesFlag() *CRUDCommandBuilder {
	b.AddBoolFlag(crudFlagUnlinkReferences, "", false,
		"Strip this ID from dependents' reference fields, then delete (does not delete dependent objects)")
	return b
}

// WithQueryFlags adds standard query flags (filter, sort, pagination)
func (b *CRUDCommandBuilder) WithQueryFlags() *CRUDCommandBuilder {
	b.AddStringArrayFlag(crudFlagFilter, "", crudHelpFilter)
	b.AddStringFlag(crudFlagSortBy, "", "", crudHelpSortBy)
	b.AddBoolFlag(crudFlagSortAsc, "", true, crudHelpSortAsc)
	b.AddIntFlag(crudFlagOffset, "", 0, crudHelpOffset)
	b.AddIntFlag(crudFlagLimit, "", 0, crudHelpLimit)
	return b
}

// ApplyBuilder merges overrides into a generated builder command.
// This facilitates incremental migration of legacy cobra.Command definitions to spec-driven builders.
func ApplyBuilder(builder *cobra.Command, overrides *cobra.Command) *cobra.Command {
	if overrides.Use != "" {
		builder.Use = overrides.Use
	}
	if overrides.Short != "" {
		builder.Short = overrides.Short
	}
	if overrides.Long != "" {
		builder.Long = overrides.Long
	}
	if overrides.Example != "" {
		builder.Example = overrides.Example
	}
	if overrides.Args != nil {
		builder.Args = overrides.Args
	}
	if overrides.RunE != nil {
		builder.RunE = overrides.RunE
	}
	if overrides.Run != nil {
		builder.Run = overrides.Run
	}
	if overrides.PreRunE != nil {
		builder.PreRunE = overrides.PreRunE
	}
	if overrides.PreRun != nil {
		builder.PreRun = overrides.PreRun
	}
	if overrides.PostRunE != nil {
		builder.PostRunE = overrides.PostRunE
	}
	if overrides.PostRun != nil {
		builder.PostRun = overrides.PostRun
	}
	if overrides.Aliases != nil {
		builder.Aliases = overrides.Aliases
	}
	builder.Hidden = overrides.Hidden
	builder.Deprecated = overrides.Deprecated
	builder.SilenceErrors = overrides.SilenceErrors
	builder.SilenceUsage = overrides.SilenceUsage
	builder.DisableFlagParsing = overrides.DisableFlagParsing
	return builder
}
