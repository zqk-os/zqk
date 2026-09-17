package object

import (
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// NewCountCmd creates a new count command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewCountCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectCountCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runCount)

	// Add count-specific flags using shared utility (query flags not in DNA yet — count_harness_flags is expected-names only)
	clipkg.AddCountFlags(cmd)

	cmd.ValidArgsFunction = kindCompletion

	// Field-aware flag completions for count (group-by + filter).
	_ = cmd.RegisterFlagCompletionFunc("group-by", completeGroupBy)
	_ = cmd.RegisterFlagCompletionFunc("filter", completeFilterField)

	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidateCountArg0
	addAllKindsFlag(cmd)

	return cmd
}

func runCount(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		if err := RequireElevatedInternal(cmd); err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		// Get group-by option
		groupBy, _ := cmd.Flags().GetString("group-by") //nolint:errcheck

		var multiKinds []string
		var singleKind string
		kindArg := ""
		if len(args) > 0 {
			kindArg = args[0]
			if list, ok := kindsListFromPRERun(cmd); ok {
				if len(list) > 1 {
					multiKinds = list
				} else {
					singleKind = list[0]
				}
			} else {
				var rerr error
				if strings.Contains(kindArg, ",") {
					multiKinds, rerr = objects.ResolveAndValidateKindsCommaSeparated(proc.ProjectRoot(), kindArg)
				} else {
					singleKind, rerr = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), kindArg)
				}
				if rerr != nil {
					return cli.Guard(cmd).Err(rerr).Return()
				}
			}
		}

		// Build filters
		filters, nsScope, err := parseCountFilters(cmd, proc)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		includeZeroCount, _ := cmd.Flags().GetBool("include-zero-count") //nolint:errcheck

		// If no kind specified or wildcard, count all discoverable kinds
		if len(args) == 0 || (len(args) > 0 && (args[0] == "*" || args[0] == "all")) {
			return cli.EnhanceError(cmd, countAllKinds(cmd, proc, filters, groupBy, includeZeroCount, nsScope))
		}

		// Check if argument contains comma-separated kinds
		if strings.Contains(kindArg, ",") {
			return cli.EnhanceError(cmd, countMultipleKinds(cmd, proc, multiKinds, filters, includeZeroCount, nsScope))
		}

		// Single kind specified
		kind := singleKind
		logging.FluentEvent(proc.Logger()).Debug("Counting objects").
			String("kind", kind).
			Int("filter_count", len(filters)).
			String("group_by", groupBy).
			Log()

		if groupBy != emptyValue {
			return cli.EnhanceError(cmd, countSingleKindWithGrouping(cmd, proc, kind, filters, groupBy, nsScope))
		}
		return cli.EnhanceError(cmd, countSingleKindWithoutGrouping(cmd, proc, kind, filters, nsScope))
	})(cmd, args)
}

// outputAllKindsCount is now a wrapper around shared utility for backward compatibility
func outputAllKindsCount(cmd *cobra.Command, counts map[string]int, format string, scopeNote string, nsScope NamespaceQueryScope) error {
	data, err := clipkg.OutputAllKindsCount(counts, format, scopeNote, attachNamespaceScopeMeta(nil, nsScope))
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	return cli.WriteOutput(cmd, data)
}
