package system

import (
	stdcontext "context"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewAggregateChangeJournalCmd creates a command to aggregate change journal entries
func NewAggregateChangeJournalCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Aggregate change journal entries into metrics",
		"Aggregate change journal entries within a time window into metrics for efficient storage.",
		"",
		"This command:",
		"  - Queries change journal entries in the specified time window",
		"  - Aggregates them into audit_aggregation_metric objects",
		"  - Marks entries as aggregated",
		"  - Optionally archives or deletes processed entries",
	).
		AddExample("Aggregate entries from last 24 hours", "%s system aggregate-change-journal --window 24h").
		AddExample("Aggregate entries from specific date range", "%s system aggregate-change-journal --start \"2025-12-01T00:00:00Z\" --end \"2025-12-02T00:00:00Z\"").
		AddExample("Aggregate and delete processed entries (use with caution)", "%s system aggregate-change-journal --window 24h --delete").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAggregateChangeJournalCommandBuilder(), &cobra.Command{
		Use:  "aggregate-change-journal",
		Args: cobra.NoArgs,
		RunE: runAggregateChangeJournal,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("window", "24h", "Time window for aggregation (e.g., 24h, 7d, 1w)")
	cmd.Flags().String("start", "", "Start time for aggregation window (ISO 8601 format)")
	cmd.Flags().String("end", "", "End time for aggregation window (ISO 8601 format, defaults to now)")
	cmd.Flags().Bool("delete", false, "Delete processed entries after aggregation (use with caution)")

	cli.AddCommonFlags(cmd)
	return cmd
}

func runAggregateChangeJournal(cmd *cobra.Command, args []string) error {
	return RunAggregateChangeJournalViaPipeline(cmd, args)
}

// executeAggregateChangeJournalCore executes the change-journal aggregation core (storage writes).
func executeAggregateChangeJournalCore(
	service *storage.ChangeJournalAggregationService,
	ctx stdcontext.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	windowStart, windowEnd time.Time,
) (*storage.ChangeJournalAggregationResult, error) {
	return service.AggregateChangeJournalEntries(ctx, secCtx, storageCtx, windowStart, windowEnd)
}
