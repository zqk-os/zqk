package workflow

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/do"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewWorkflowCmd creates the workflow command group.
func NewWorkflowCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowCommandBuilder()
	cmd.AddCommand(NewNextCmd())
	cmd.AddCommand(NewWhatsNextCmd())
	cmd.AddCommand(NewCheckCmd())
	cmd.AddCommand(NewVDSCmd())
	cmd.AddCommand(NewAddCmd())
	cmd.AddCommand(NewCoachCmd())
	cmd.AddCommand(NewGenTracePipelineCmd())
	cmd.AddCommand(NewLinkCmd())
	cmd.AddCommand(do.NewDoCmd())
	return cmd
}

// processorContexts extracts standard execution contexts from a CLI processor.
func processorContexts(proc *cli.Processor) (context.Context, storage.ObjectStorageProvider, *pkgctx.SecurityContext) {
	return proc.OperationContext(), proc.Storage(), proc.SecurityContext()
}
