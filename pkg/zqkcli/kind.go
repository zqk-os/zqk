package internal

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// Internal kinds that support "internal <kind> fields" (must match Cobra subcommand names).
var internalKindsForFields = []string{objects.KindObjectSpec, objects.KindLifecycle}

// NewInternalKindCmd creates the generic "<kind>" command group for internal.
// RegisterDynamicInternalKindCommands adds per-kind subcommands so "internal object_spec fields" is routable.
func NewInternalKindCmd() *cobra.Command {
	return newInternalKindCmdWithUse("<kind>", cobra.ExactArgs(1), true, "")
}

func newInternalKindCmdWithUse(use string, args cobra.PositionalArgs, useKindArg bool, kindName string) *cobra.Command {
	kindCmd := clipkg.NewCommandBuilder(use).
		WithShort("Operations for a specific internal object kind").
		WithLong("Operations for a specific internal object kind. Use 'fields' subcommand to explore available fields.").
		WithArgs(args).
		WithValidArgsFunction(internalKindCompletion).
		Build()
	kindCmd.DisableFlagParsing = false

	kindCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		k := kindName
		if useKindArg && len(args) > 0 {
			k = args[0]
		}
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string)
		}
		cmd.Annotations[objects.FieldKeyKind] = k
		for _, child := range cmd.Commands() {
			if child.Annotations == nil {
				child.Annotations = make(map[string]string)
			}
			child.Annotations[objects.FieldKeyKind] = k
		}
		return nil
	}

	kindCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	kindFieldsCmd := newInternalKindFieldsCmd()
	kindCmd.AddCommand(kindFieldsCmd)

	return kindCmd
}

// NewInternalKindCmdForKind creates a kind subcommand so "internal object_spec fields" matches.
func NewInternalKindCmdForKind(kind string) *cobra.Command {
	return newInternalKindCmdWithUse(kind, cobra.NoArgs, false, kind)
}

// RegisterDynamicInternalKindCommands adds subcommands for internal kinds (e.g. object_spec, lifecycle)
// so "internal object_spec fields" is routable. Uses exact name match so we add "object_spec" even
// when a generic "<kind>" subcommand exists (Find would return <kind> and we would skip adding).
func RegisterDynamicInternalKindCommands(internalCmd *cobra.Command) {
	needed := false
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			if arg == "internal" || arg == "completion" || arg == "__complete" {
				needed = true
				break
			}
		}
	}
	if !needed {
		return
	}

	have := make(map[string]bool)
	for _, c := range internalCmd.Commands() {
		have[c.Name()] = true
	}
	for _, kind := range internalKindsForFields {
		if have[kind] {
			continue
		}
		internalCmd.AddCommand(NewInternalKindCmdForKind(kind))
		have[kind] = true
	}
}

// newInternalKindFieldsCmd creates a fields command that works as a subcommand of a kind
// Usage: "internal <kind> fields [flags]"
func newInternalKindFieldsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List available fields for this internal object kind",
		"List all available fields for this internal object kind, grouped by:",
		"  - Common fields (inherited from base_object/auditable, available for all objects)",
		"  - Specialized fields (specific to this object kind)",
		"",
		"Fields are sorted alphabetically and include:",
		"  - Field name",
		"  - Type (string, integer, list, etc.)",
		"  - Semantic type (statement, reference, quantity, etc.)",
		"  - Traits (listable, readable, writable, filterable, sortable, etc.)",
		"  - Required status",
		"  - Description",
		"",
		"This command is accessed via the kind command group, making it convenient to explore",
		"internal object structure: '%s internal <kind> fields'",
	).
		AddExample("List all fields", "%s internal object_spec fields").
		AddExample("List only filterable fields", "%s internal object_spec fields --filterable").
		AddExample("List only sortable fields", "%s internal object_spec fields --sortable").
		AddExample("List only groupable fields", "%s internal object_spec fields --groupable").
		AddExample("List only specialized fields", "%s internal object_spec fields --specialized-only").
		ExcludeCommonFlags()

	fieldsCmd := clipkg.NewCommandBuilder("fields [flags]").
		WithArgs(cobra.NoArgs).
		WithRunE(runInternalKindFields).
		Build()

	helpBuilder.ApplyToCommand(fieldsCmd)

	clipkg.AddFieldsFlags(fieldsCmd, false)

	ensureCmdAnnotations(fieldsCmd)
	fieldsCmd.Annotations[AnnotationKindValidate] = KindValidateFieldsParentKind

	return fieldsCmd
}

// runInternalKindFields runs the fields command as a subcommand of a kind
// The kind is extracted from the parent command's arguments
func runInternalKindFields(cmd *cobra.Command, args []string) error {
	// Get the kind from the parent command (the kind command)
	parent := cmd.Parent()
	if parent == nil {
		return errfmt.Errorf("fields command must be used as a subcommand of a kind")
	}

	// First try to get from command's own annotation (set in PersistentPreRunE)
	var kindArg string
	if kindVal, ok := cmd.Annotations[objects.FieldKeyKind]; ok {
		kindArg = kindVal
	}

	// Fallback: try to get from parent's annotation
	if kindArg == emptyValue {
		if kindVal, ok := parent.Annotations[objects.FieldKeyKind]; ok {
			kindArg = kindVal
		}
	}

	// Last resort: parse from command path: internal <kind> fields
	if kindArg == emptyValue {
		cmdPath := strings.Split(cmd.CommandPath(), " ")
		if len(cmdPath) >= 3 && cmdPath[0] == "internal" {
			kindArg = cmdPath[1]
		}
	}

	if kindArg == emptyValue {
		return errfmt.Errorf("kind is required (usage: %s internal <kind> fields)", paths.CLICommandName)
	}

	// Create processor
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to create processor").Wrap(err)
	}

	kind, ok := kindCanonicalFromInternalPRERun(cmd)
	if !ok {
		var rerr error
		kind, rerr = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), kindArg)
		if rerr != nil {
			return errfmt.Newf("invalid object kind").Wrap(rerr)
		}
	}

	// Get field registry
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to load field registry", err).Log()
		return errfmt.Newf("failed to load field registry").Wrap(err)
	}

	// Get fields for the kind
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to get fields for kind", err).
			String(objects.FieldKeyKind, kind).
			Log()
		return errfmt.Newf("failed to get fields for kind %s", kind).Wrap(err)
	}

	opts, err := clipkg.ParseFieldsFlags(cmd)
	if err != nil {
		return err
	}
	hasFilter := opts.Filterable || opts.Sortable || opts.Groupable || opts.SpecializedOnly

	if !hasFilter {
		// Default: show all fields with common/specialized segregation (same as object)
		switch opts.Format {
		case "json":
			return cli.FormatOutputAs(cmd, cli.FormatJSON, clipkg.KindFieldsSegregatedData(kindFields))
		case "yaml":
			return cli.FormatOutputAs(cmd, cli.FormatYAML, clipkg.KindFieldsSegregatedData(kindFields))
		default:
			return cli.WriteOutput(cmd, clipkg.FormatKindFieldsSegregatedTable(kindFields))
		}
	}

	// Filter applied: show filtered list
	filteredFields := filterFields(kindFields, opts.Filterable, opts.Sortable, opts.Groupable, opts.SpecializedOnly, proc.Logger())
	format := proc.Format()
	return outputFields(cmd, filteredFields, kind, format)
}

// filterFields filters fields based on flags
func filterFields(kindFields *objects.KindFields, filterable, sortable, groupable, specializedOnly bool, logger *logging.EventLogger) []objects.FieldInfo {
	var result []objects.FieldInfo

	if specializedOnly {
		return kindFields.SpecializedFields
	}

	if filterable {
		fieldNames, err := objects.GenerateFilterableFields(kindFields.Kind)
		if err != nil {
			logging.FluentEvent(logger).Error("Failed to get filterable fields", err).Log()
			return nil
		}
		fieldMap := buildFieldMap(kindFields)
		for _, name := range fieldNames {
			if field, ok := fieldMap[name]; ok {
				result = append(result, field)
			}
		}
		return result
	}

	if sortable {
		fieldNames, err := objects.GenerateSortableFields(kindFields.Kind)
		if err != nil {
			logging.FluentEvent(logger).Error("Failed to get sortable fields", err).Log()
			return nil
		}
		fieldMap := buildFieldMap(kindFields)
		for _, name := range fieldNames {
			if field, ok := fieldMap[name]; ok {
				result = append(result, field)
			}
		}
		return result
	}

	if groupable {
		fieldNames, err := objects.GenerateGroupableFields(kindFields.Kind)
		if err != nil {
			logging.FluentEvent(logger).Error("Failed to get groupable fields", err).Log()
			return nil
		}
		fieldMap := buildFieldMap(kindFields)
		for _, name := range fieldNames {
			if field, ok := fieldMap[name]; ok {
				result = append(result, field)
			}
		}
		return result
	}

	// Return all fields
	return kindFields.AllFields
}

// buildFieldMap builds a map of field names to FieldInfo
func buildFieldMap(kindFields *objects.KindFields) map[string]objects.FieldInfo {
	fieldMap := make(map[string]objects.FieldInfo)
	for i := range kindFields.AllFields {
		field := &kindFields.AllFields[i]
		fieldMap[field.Name] = *field
	}
	return fieldMap
}

// outputFields outputs fields in the requested format
func outputFields(cmd *cobra.Command, fields []objects.FieldInfo, kind string, format cli.OutputFormat) error {
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		data := map[string]any{
			objects.FieldKeyKind: kind,
			"fields":             convertFieldsToMaps(fields),
		}
		return cli.FormatOutput(cmd, data)
	default:
		return outputFieldsTable(cmd, fields, kind)
	}
}

// outputFieldsTable outputs fields in table format
func outputFieldsTable(cmd *cobra.Command, fields []objects.FieldInfo, kind string) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Fields for '%s':\n\n", kind)
	buf.WriteString(strings.Repeat("-", 60) + "\n")

	for i := range fields {
		field := &fields[i]
		fmt.Fprintf(&buf, "  %s", field.Name)
		if field.Type != emptyValue {
			fmt.Fprintf(&buf, " (%s)", field.Type)
		}
		if len(field.Traits) > 0 {
			fmt.Fprintf(&buf, " [%s]", strings.Join(field.Traits, ", "))
		}
		if field.Required {
			buf.WriteString(" (required)")
		}
		if field.Description != emptyValue {
			fmt.Fprintf(&buf, "\n    %s", field.Description)
		}
		buf.WriteString("\n")
	}

	return cli.WriteOutput(cmd, buf.Bytes())
}

// convertFieldsToMaps converts FieldInfo slices to maps for JSON/YAML output
func convertFieldsToMaps(fields []objects.FieldInfo) []map[string]any {
	result := make([]map[string]any, len(fields))
	for i := range fields {
		field := &fields[i]
		m := map[string]any{
			objects.FieldKeyName: field.Name,
		}
		if field.Type != emptyValue {
			m[objects.FieldKeyType] = field.Type
		}
		if len(field.Traits) > 0 {
			m["traits"] = field.Traits
		}
		if field.Required {
			m["required"] = true
		}
		if field.Description != emptyValue {
			m[objects.FieldKeyDescription] = field.Description
		}
		result[i] = m
	}
	return result
}

// internalKindCompletion provides completion for internal object kinds
func internalKindCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	kinds, err := registry.GetAllKinds()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Filter kinds that start with toComplete
	var matches []string
	for _, kind := range kinds {
		if len(args) == 0 && (toComplete == emptyValue || strings.HasPrefix(kind, toComplete)) {
			matches = append(matches, kind)
		}
	}

	return matches, cobra.ShellCompDirectiveNoFileComp
}

// internalCompleteGroupBy suggests groupable fields for the current internal kind.
func internalCompleteGroupBy(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	kind := resolveInternalKindForCompletion(cmd, args)
	if kind == emptyValue {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	projectRoot := cli.ResolveProjectRoot(".")
	idx := objects.TryLoadSpecIndexForProjectRoot(projectRoot)
	if idx == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	candidates := clipkg.BuildFieldCompletionCandidates(idx, objects.GetCanonicalKind(kind), func(f objects.SpecFieldSummary) bool {
		return f.Groupable && strings.HasPrefix(f.Name, toComplete)
	})
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.Description != emptyValue {
			out = append(out, fmt.Sprintf("%s\t%s", c.Name, c.Description))
		} else {
			out = append(out, c.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// internalCompleteSortBy suggests sortable fields for the current internal kind.
func internalCompleteSortBy(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	kind := resolveInternalKindForCompletion(cmd, args)
	if kind == emptyValue {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	projectRoot := cli.ResolveProjectRoot(".")
	idx := objects.TryLoadSpecIndexForProjectRoot(projectRoot)
	if idx == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	candidates := clipkg.BuildFieldCompletionCandidates(idx, objects.GetCanonicalKind(kind), func(f objects.SpecFieldSummary) bool {
		return f.Sortable && strings.HasPrefix(f.Name, toComplete)
	})
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.Description != emptyValue {
			out = append(out, fmt.Sprintf("%s\t%s", c.Name, c.Description))
		} else {
			out = append(out, c.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// internalCompleteFilterField suggests filterable fields for the current internal kind.
func internalCompleteFilterField(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	kind := resolveInternalKindForCompletion(cmd, args)
	if kind == emptyValue {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	projectRoot := cli.ResolveProjectRoot(".")
	idx := objects.TryLoadSpecIndexForProjectRoot(projectRoot)
	if idx == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// If an operator is present, first try to complete enum values for that field.
	sepIdx := strings.IndexAny(toComplete, "=:")
	if sepIdx >= 0 {
		fieldName := toComplete[:sepIdx]
		valuePrefix := ""
		if sepIdx+1 < len(toComplete) {
			valuePrefix = toComplete[sepIdx+1:]
		}

		enumValues := clipkg.BuildEnumValueCompletions(idx, objects.GetCanonicalKind(kind), fieldName, valuePrefix)
		if len(enumValues) > 0 {
			suggestions := make([]string, 0, len(enumValues))
			sep := string(toComplete[sepIdx])
			for _, v := range enumValues {
				suggestions = append(suggestions, fmt.Sprintf("%s%s%s", fieldName, sep, v))
			}
			return suggestions, cobra.ShellCompDirectiveNoFileComp
		}
	}

	// Otherwise complete field names up to the first operator/value separator.
	prefix := toComplete
	if sepIdx >= 0 {
		prefix = toComplete[:sepIdx]
	}

	candidates := clipkg.BuildFieldCompletionCandidates(idx, objects.GetCanonicalKind(kind), func(f objects.SpecFieldSummary) bool {
		return f.Filterable && (prefix == emptyValue || strings.HasPrefix(f.Name, prefix))
	})
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.Description != emptyValue {
			out = append(out, fmt.Sprintf("%s\t%s", c.Name, c.Description))
		} else {
			out = append(out, c.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// resolveInternalKindForCompletion mirrors resolveCurrentKindForCompletion but for the "internal"
// command tree. It checks annotations and, if needed, parses the command path.
func resolveInternalKindForCompletion(cmd *cobra.Command, args []string) string {
	if k, ok := cmd.Annotations[objects.FieldKeyKind]; ok && k != emptyValue {
		return k
	}
	if parent := cmd.Parent(); parent != nil {
		if k, ok := parent.Annotations[objects.FieldKeyKind]; ok && k != emptyValue {
			return k
		}
	}
	cmdPath := strings.Split(cmd.CommandPath(), " ")
	if len(cmdPath) >= 3 && cmdPath[0] == "internal" {
		return cmdPath[1]
	}
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
