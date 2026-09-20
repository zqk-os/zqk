package object

import (
	"fmt"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewPPlanCurrentCmd creates the current subcommand
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewPPlanCurrentCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectPplanCurrentCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runPPlanCurrent)

	return cmd
}

func runPPlanCurrent(cmd *cobra.Command, args []string) error {
	logger := logging.GetLoggerFromContext(cmd.Context())

	// Get context from command (same pattern as list.go)
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return cli.Guard(cmd).Err(errfmt.Errorf("failed to get context")).Return()
	}

	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}

	storageProvider, err := cli.GetObjectStorageForCommand(cmd, projectRoot)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to get storage: %w").Return()
	}

	// Find current priority plan
	currentPlan, err := findCurrentPriorityPlan(cmd.Context(), storageProvider)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to find current priority plan: %w").Return()
	}

	logging.FluentEvent(logger).Info(fmt.Sprintf("Current priority plan: %s (status: %s)", currentPlan.ID, currentPlan.Status)).Log()

	// Build filter for backlog items (exclude terminal statuses: complete, archived, rejected)
	filters := map[string]any{
		pplanFieldPriorityRef: currentPlan.ID,
		pplanFieldStatus:      statusFilterExcludingTerminal(),
	}

	// Parse additional filters from flags
	filterFlags, err := cmd.Flags().GetStringArray("filter")
	if err != nil {
		filterFlags = []string{}
	}
	for _, filterStr := range filterFlags {
		field, value, err := ParseFilterString(filterStr)
		if err != nil {
			return cli.EnhanceError(cmd, errfmt.Errorf("invalid filter: %s: %w", filterStr, err))
		}
		filters[field] = value
	}

	// Build list filter
	filter := storage.ListFilter{
		Kind:    pplanKindBacklogItem,
		Filters: filters,
		GroupBy: pplanSortPriorityTier,
	}

	// Parse sort options
	sortBy, err := cmd.Flags().GetString("sort-by")
	if err != nil {
		sortBy = ""
	}
	if sortBy == emptyValue {
		sortBy = pplanSortPriorityTier // Default sort
	}
	sortAsc, err := cmd.Flags().GetBool("sort-asc")
	if err != nil {
		sortAsc = false
	}
	filter.SortBy = sortBy
	filter.SortAsc = sortAsc

	// Execute list
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := ctx.GetStorageContext()
	storageCtx.EnableGrouping = true // Enable grouping for pplan current
	result, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, filter)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to list backlog items: %w").Return()
	}

	// Output results using the same logic as list command
	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return outputListStructured(cmd, result)
	case cli.FormatTable:
		return outputListTable(cmd, result, pplanKindBacklogItem, "", nil)
	default:
		return outputListTable(cmd, result, pplanKindBacklogItem, "", nil)
	}
}
