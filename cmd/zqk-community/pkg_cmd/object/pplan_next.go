package object

import (
	"fmt"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewPPlanNextCmd creates the next subcommand
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewPPlanNextCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectPplanNextCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runPPlanNext)

	return cmd
}

func runPPlanNext(cmd *cobra.Command, args []string) error {
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

	// Get current plan
	currentPlan, err := findCurrentPriorityPlan(cmd.Context(), storageProvider)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to find current priority plan: %w").Return()
	}

	// Get all active plans
	allPlans, err := getAllActivePriorityPlans(cmd.Context(), storageProvider)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to get all priority plans: %w").Return()
	}

	// Find current plan index
	currentIndex := -1
	for i, plan := range allPlans {
		if plan.ID == currentPlan.ID {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return cli.Guard(cmd).Err(errfmt.Errorf("current plan %s not found in active plans", currentPlan.ID)).Return()
	}

	// Get next plan (wrap around)
	nextIndex := (currentIndex + 1) % len(allPlans)
	nextPlan := allPlans[nextIndex]

	logging.FluentEvent(logger).Info(fmt.Sprintf("Next priority plan: %s (status: %s)", nextPlan.ID, nextPlan.Status)).Log()

	// Build filter for backlog items (exclude terminal statuses: complete, archived, rejected)
	filters := map[string]any{
		pplanFieldPriorityRef: nextPlan.ID,
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
	}

	// Parse sort options
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	sortBy, _ := cmd.Flags().GetString("sort-by")
	if sortBy == emptyValue {
		sortBy = pplanSortStatus
	}
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	sortAsc, _ := cmd.Flags().GetBool("sort-asc")
	filter.SortBy = sortBy
	filter.SortAsc = sortAsc

	// Execute list
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := ctx.GetStorageContext()
	result, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, filter)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to list backlog items: %w").Return()
	}

	// Output results
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
