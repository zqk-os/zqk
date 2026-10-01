package object

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objectget"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewKindCmd creates a command group for a specific object kind (generic "<kind>" placeholder).
// Cobra matches by name, so "object backlog_item fields" only works when per-kind commands
// are registered via RegisterDynamicKindCommands.
func NewKindCmd() *cobra.Command {
	return newKindCmdWithUse("<kind>", cobra.ExactArgs(1), true, "")
}

// kindCommandShort returns a cobra Short for a kind from the object_spec description
// (first sentence/line, truncated). Falls back to a kind-qualified stub — never the
// identical "Operations for a specific object kind" string for every kind.
// inventory membrane honesty / discoverability.
func kindCommandShort(kind string) string {
	const maxShort = 96
	if kind == "" || kind == "<kind>" {
		return "Operations for a specific object kind (use object <kind> …)"
	}
	if loader := objects.GetGlobalSpecLoader(); loader != nil {
		if spec, err := loader.LoadSpec(kind); err == nil && spec != nil {
			if s := firstHelpSentence(spec.Description, maxShort); s != "" {
				return s
			}
		}
	}
	return kind + " — list/get/create/update/delete (schema registered; instances may be empty)"
}

func firstHelpSentence(desc string, maxLen int) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	// Prefer the first substantive line (skip "Lifecycle: …" pointers common in specs).
	var line string
	for _, raw := range strings.Split(desc, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(strings.ToLower(raw), "lifecycle:") {
			continue
		}
		line = raw
		break
	}
	if line == "" {
		line = strings.TrimSpace(strings.Split(desc, "\n")[0])
	}
	desc = line
	for _, sep := range []string{". ", "? ", "! "} {
		if i := strings.Index(desc, sep); i >= 0 {
			desc = desc[:i+1]
			break
		}
	}
	desc = strings.TrimSpace(desc)
	if maxLen > 0 {
		r := []rune(desc)
		if len(r) > maxLen-1 {
			desc = string(r[:maxLen-1]) + "…"
		}
	}
	return desc
}

// newKindCmdWithUse creates a kind command with the given Use and args. For dynamic kinds,
// useKindArg is false and kindName is set so PersistentPreRunE stores that kind in annotations.
func newKindCmdWithUse(use string, args cobra.PositionalArgs, useKindArg bool, kindName string) *cobra.Command {
	short := kindCommandShort(kindName)
	if kindName == "" && use == "<kind>" {
		short = kindCommandShort("<kind>")
	}
	long := short + "\n\nSubcommands: fields, list, get, create, update, delete.\n" +
		"Help lists registered schemas (object_specs). list/count show CAS/stream instances — often empty when only the schema exists (e.g. team_configuration, object_spec under _internal)."
	kindCmd := &cobra.Command{
		Use:                use,
		Short:              short,
		Long:               long,
		Args:               args,
		ValidArgsFunction:  kindCompletion,
		DisableFlagParsing: false,
	}

	// Set PersistentPreRunE to store the kind for subcommands
	kindCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		k := kindName
		if useKindArg && len(args) > 0 {
			k = args[0]
		}
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string)
		}
		cmd.Annotations[objects.FieldKeyKind] = k
		cmd.Annotations[AnnotationKindCanonical] = k
		for _, child := range cmd.Commands() {
			if child.Annotations == nil {
				child.Annotations = make(map[string]string)
			}
			child.Annotations[objects.FieldKeyKind] = k
			child.Annotations[AnnotationKindCanonical] = k
		}
		return nil
	}

	kindCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	kindFieldsCmd := newKindFieldsCmd()
	kindCmd.AddCommand(kindFieldsCmd)

	// Add kind-specific CRUD subcommands for consistency and discovery (e.g. zqk object priority_plan list)
	kindCmd.AddCommand(NewListCmd())
	kindCmd.AddCommand(NewGetCmd())
	kindCmd.AddCommand(NewCreateCmd())
	kindCmd.AddCommand(NewUpdateCmd())
	kindCmd.AddCommand(NewDeleteCmd())

	return kindCmd
}

// NewKindCmdForKind creates a kind subcommand with the given kind name so Cobra matches
// "object backlog_item fields" when the first argument is that kind (e.g. backlog_item).
func NewKindCmdForKind(kind string) *cobra.Command {
	return newKindCmdWithUse(kind, cobra.NoArgs, false, kind)
}

// RegisterKindCommandsForKinds adds a subcommand to objectCmd for each kind in kinds.
// Used by tests when the global registry was loaded with ZQK_TEST_ROOT and the test
// needs to ensure kind subcommands are present (e.g. object backlog_item fields).
// Uses exact name match so we add a per-kind command (e.g. backlog_item) even when a generic "<kind>" exists.
func RegisterKindCommandsForKinds(objectCmd *cobra.Command, kinds []string) {
	have := make(map[string]bool)
	for _, c := range objectCmd.Commands() {
		have[c.Name()] = true
	}
	for _, kind := range kinds {
		if have[kind] {
			continue
		}
		kc := NewKindCmdForKind(kind)
		kc.GroupID = objectHelpGroupKinds
		// Keep kinds routable but out of the flat -h scrape surface; use fields --list-kinds.
		kc.Hidden = true
		objectCmd.AddCommand(kc)
		have[kind] = true
	}
}

// RegisterDynamicKindCommands loads the field registry and adds a subcommand to objectCmd
// for each known kind (e.g. backlog_item, goal) so "object <kind> fields" is routable.
// If the registry was created before ZQK_TEST_ROOT was set (e.g. in tests), retries once
// using findSpecsDir() so kind subcommands are added.
func RegisterDynamicKindCommands(objectCmd *cobra.Command) {
	// PERF: Skip eager evaluation of all kinds unless we actually need to route a kind command.
	// Cobra evaluates the command tree on startup. Kinds are Hidden=true, so they aren't
	// needed for help menus, only for routing commands like `zqk object backlog_item fields`.
	needed := false
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			if arg == "object" || arg == "internal" || arg == "completion" || arg == "__complete" {
				needed = true
				break
			}
		}
	}
	if !needed {
		return
	}

	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		if registry.TryReloadFromFindSpecsDir() {
			_ = registry.LoadFields()
		} else {
			return
		}
	}
	kinds, err := registry.GetAllKinds()
	if err != nil || len(kinds) == 0 {
		if registry.TryReloadFromFindSpecsDir() {
			_ = registry.LoadFields()
			kinds, err = registry.GetAllKinds()
		}
	}
	if err != nil || len(kinds) == 0 {
		return
	}
	// Use RegisterKindCommandsForKinds so we add by exact name; Find([]string{kind}) would
	// return the generic <kind> command and we would skip adding (no per-kind subcommands).

	attachDiscoveredKindCommands(objectCmd, kinds)
}

// packKindRegistrars attach kinds each linked pack owns. The composition root adds them.
// The kernel strips those kinds from its own list and does not know their names.
var packKindRegistrars []func(objectCmd *cobra.Command)

// SetPackKindRegistrar replaces the pack command hooks.
// A nil registrar clears them.
func SetPackKindRegistrar(fn func(objectCmd *cobra.Command)) {
	if fn == nil {
		packKindRegistrars = nil
		return
	}
	packKindRegistrars = []func(objectCmd *cobra.Command){fn}
}

// AddPackKindRegistrar records another linked pack's kind commands.
func AddPackKindRegistrar(fn func(objectCmd *cobra.Command)) {
	if fn == nil {
		return
	}
	packKindRegistrars = append(packKindRegistrars, fn)
}

func attachDiscoveredKindCommands(objectCmd *cobra.Command, kinds []string) {
	RegisterKindCommandsForKinds(objectCmd, objects.WithoutKinds(kinds, objects.PackOwnedKinds()))
	for _, fn := range packKindRegistrars {
		fn(objectCmd)
	}
}

// runKindDefault is handled inline in NewKindCmd

// newKindFieldsCmd creates a fields command that works as a subcommand of a kind
// Usage: "object <kind> fields [flags]"
func newKindFieldsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List available fields for this object kind",
		"List all available fields for this object kind, grouped by:",
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
		"object structure: '%s object <kind> fields'",
	).
		AddExample("List all fields", "%s object backlog_item fields").
		AddExample("List only filterable fields", "%s object backlog_item fields --filterable").
		AddExample("List only sortable fields", "%s object backlog_item fields --sortable").
		AddExample("List only groupable fields", "%s object backlog_item fields --groupable").
		AddExample("List only specialized fields", "%s object backlog_item fields --specialized-only").
		ExcludeCommonFlags()

	fieldsCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectFieldsCommandBuilder(), &cobra.Command{
		Use:  "fields [flags]",
		Args: cobra.NoArgs,
		RunE: runKindFields,
	})

	helpBuilder.ApplyToCommand(fieldsCmd)

	clipkg.AddFieldsFlags(fieldsCmd, false)

	ensureCmdAnnotations(fieldsCmd)
	fieldsCmd.Annotations[AnnotationKindValidate] = KindValidateFieldsParentKind

	return fieldsCmd
}

// runKindFields runs the fields command as a subcommand of a kind
// The kind is extracted from the parent command's arguments
func runKindFields(cmd *cobra.Command, args []string) error {
	// Get the kind from the parent command (the kind command)
	parent := cmd.Parent()
	if parent == nil {
		return cli.Guard(cmd).Require(false, "fields command must be used as a subcommand of a kind").Return()
	}

	// In Cobra, when a parent command has arguments, we need to access them differently
	// The parent command's args are available during execution, but we need to get them
	// from the command's context or parse from the command path

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

	// Last resort: parse from command path: object <kind> fields or internal <kind> fields
	if kindArg == emptyValue {
		cmdPath := strings.Split(cmd.CommandPath(), " ")
		if len(cmdPath) >= 3 {
			if cmdPath[0] == "object" || cmdPath[0] == "internal" {
				kindArg = cmdPath[1]
			}
		}
	}

	if kindArg == emptyValue {
		return cli.Guard(cmd).Requiref(false, "kind is required (usage: %s object <kind> fields)", paths.CLICommandName).Return()
	}

	kind, ok := kindCanonicalFromPRERun(cmd)
	if !ok {
		var err error
		kind, err = objects.ResolveAndValidateKindForProject(cli.ResolveProjectRoot("."), kindArg)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}
	}

	// Now run the fields logic with the extracted kind
	logger := logging.GetLoggerFromContext(cmd.Context())
	registry := objects.GetGlobalFieldRegistry()
	flags, err := parseFieldsFlags(cmd)
	if err != nil {
		return err
	}

	// Load fields if not already loaded
	if err := registry.LoadFields(); err != nil {
		logging.FluentEvent(logger).Error("Failed to load fields", err).Log()
		return cli.Guard(cmd).Err(err).Wrapf("failed to load fields: %w").Return()
	}

	// Get field information
	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil {
		logging.FluentEvent(logger).Error("Failed to get fields for kind", err).
			String("kind", kind).
			Log()
		return cli.EnhanceError(cmd, errfmt.Errorf("failed to get fields for kind %s: %w", kind, err))
	}

	// Get fields to show based on flags
	fieldsToShow, err := getFieldsToShow(flags, kind, logger, kindFields)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}

	// Output fields in requested format
	if fieldsToShow == nil {
		// Show all fields grouped
		return outputFieldsByFormat(cmd, flags.Format, kindFields, nil)
	}
	// Show filtered fields
	return outputFieldsByFormat(cmd, flags.Format, nil, fieldsToShow)
}

// kindCompletion provides completion for object kinds
func kindCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

// completeGroupBy suggests groupable fields for the current kind (if one is present).
func completeGroupBy(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	kind := resolveCurrentKindForCompletion(cmd, args)
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
		// Cobra expects "value\tdescription" for flag completion item descriptions.
		if c.Description != emptyValue {
			out = append(out, fmt.Sprintf("%s\t%s", c.Name, c.Description))
		} else {
			out = append(out, c.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeSortBy suggests sortable fields for the current kind (if one is present).
func completeSortBy(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	kind := resolveCurrentKindForCompletion(cmd, args)
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

// completeFilterField suggests filterable fields for the current kind. It is registered
// for the "filter" flag; Cobra passes the entire flag value being completed, so we only
// complete the field name prefix before any operator/value.
func completeFilterField(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	kind := resolveCurrentKindForCompletion(cmd, args)
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

// completeProjectFieldsForCanonicalKind suggests top-level instance field names for --fields for a resolved kind.
func completeProjectFieldsForCanonicalKind(kind, toComplete string) ([]string, cobra.ShellCompDirective) {
	kind = objects.GetCanonicalKind(kind)
	if kind == emptyValue {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	before := ""
	prefix := toComplete
	if i := strings.LastIndex(toComplete, ","); i >= 0 {
		before = toComplete[:i+1]
		prefix = strings.TrimSpace(toComplete[i+1:])
	}

	projectRoot := cli.ResolveProjectRoot(".")
	idx := objects.TryLoadSpecIndexForProjectRoot(projectRoot)
	if idx == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	candidates := clipkg.BuildFieldCompletionCandidates(idx, kind, func(f objects.SpecFieldSummary) bool {
		return prefix == emptyValue || strings.HasPrefix(f.Name, prefix)
	})
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		s := before + c.Name
		if c.Description != emptyValue {
			out = append(out, fmt.Sprintf("%s\t%s", s, c.Description))
		} else {
			out = append(out, s)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeListProjectFields suggests top-level instance field names for --fields (hybrid projection).
// Supports comma-separated values: completes the segment after the last comma.
func completeListProjectFields(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	kind := resolveCurrentKindForCompletion(cmd, args)
	return completeProjectFieldsForCanonicalKind(kind, toComplete)
}

// completeGetProjectFields resolves kind from the object ID positional (prefix inference) then completes like list.
func completeGetProjectFields(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var kind string
	if len(args) > 0 && args[0] != emptyValue {
		kind = objectget.InferKindFromObjectID(cli.ResolveProjectRoot("."), args[0])
	}
	return completeProjectFieldsForCanonicalKind(kind, toComplete)
}

// resolveCurrentKindForCompletion tries to determine the current kind in scope for a completion
// callback, using annotations (set in PersistentPreRunE) and, as a last resort, parsing the
// command path (e.g. "object backlog_item list").
func resolveCurrentKindForCompletion(cmd *cobra.Command, args []string) string {
	// Annotation on this command.
	if k, ok := cmd.Annotations[objects.FieldKeyKind]; ok && k != emptyValue {
		return k
	}
	// Annotation on parent.
	if parent := cmd.Parent(); parent != nil {
		if k, ok := parent.Annotations[objects.FieldKeyKind]; ok && k != emptyValue {
			return k
		}
	}
	// Fall back to parsing command path.
	cmdPath := strings.Split(cmd.CommandPath(), " ")
	if len(cmdPath) >= 3 {
		if cmdPath[0] == "object" || cmdPath[0] == "internal" {
			return cmdPath[1]
		}
	}
	// As a last resort, if a positional kind arg is present for list/count-style commands.
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
