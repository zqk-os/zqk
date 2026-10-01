package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objectget"
)

// NewBulkGetCmd creates a bulk get command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewBulkGetCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectBulkGetCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runBulkGet)

	return cmd
}

func runBulkGet(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ids, err := clipkg.LoadIDsFromFlags(cmd, proc.Logger())
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}
		return executeBulkGet(cmd, ids, proc)
	})(cmd, args)
}

func executeBulkGet(cmd *cobra.Command, ids []string, proc *cli.Processor) error {
	var err error
	_ = err

	viewName, _ := cmd.Flags().GetString("view")
	if viewName == emptyValue {
		viewName = objectget.ViewDefault
	}

	linkHydrationRaw, _ := cmd.Flags().GetString("link-hydration")
	hydration, parseErr := objectget.ParseLinkHydration(linkHydrationRaw)
	if parseErr != nil {
		return cli.Guard(cmd).Err(parseErr).Return()
	}

	result, err := proc.Storage().BulkGet(proc.OperationContext(), proc.SecurityContext(), ids)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Bulk get failed", err).Log()
		return cli.Guard(cmd).Err(err).Wrapf("bulk get failed: %w").Return()
	}

	// Apply overlays + view projection per returned object.
	for i := range result.Results {
		item := result.Results[i]

		applyObjectGetOverlays(proc, item, viewName, hydration)
		result.Results[i] = applyObjectViewProjection(viewName, item)
	}

	projectFields, perr := clipkg.FieldsFromCmd(cmd)
	if perr != nil {
		return cli.Guard(cmd).Err(perr).Return()
	}
	applyHybridProjectionToBulkResult(result, projectFields)

	// Output results (format respects context precedence: system -> user -> project -> command)
	format := string(proc.Format())
	outputBulkResult(cmd, result, format, "get")

	return nil
}
