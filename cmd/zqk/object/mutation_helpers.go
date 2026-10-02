package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// processDryRunResult processes the result of a dry-run invocation and formats output if handled.
func processDryRunResult(cmd *cobra.Command, handled bool, result any, err error) (bool, error) {
	if err != nil {
		return handled, err
	}
	if handled && result != nil {
		return true, cli.FormatOutput(cmd, result)
	}
	return handled, nil
}

// enforceOverrideFrictionFromFlags checks the reason-code flag and enforces friction when --override is active.
func enforceOverrideFrictionFromFlags(cmd *cobra.Command, proc *cli.Processor, id, objKind, missingReasonMsg string) error {
	reasonCode := emptyValue
	if cmd != nil && cmd.Flags() != nil && cmd.Flags().Lookup("reason-code") != nil {
		reasonCode, _ = cmd.Flags().GetString("reason-code")
	}
	if reasonCode == emptyValue {
		if missingReasonMsg == "" {
			missingReasonMsg = "--reason-code is required when using --override"
		}
		return cli.Guard(cmd).Require(false, missingReasonMsg).Return()
	}
	if proc == nil {
		return nil
	}
	return clipkg.EnforceOverrideFriction(cmd, proc.OperationContext(), proc.SecurityContext(), proc.Storage(), id, objKind, reasonCode)
}

// configureRelaxedMode enables batch relaxed mode when --relaxed is present.
func configureRelaxedMode(cmd *cobra.Command, proc *cli.Processor) {
	if cmd == nil || proc == nil {
		return
	}
	relaxed, err := cmd.Flags().GetBool("relaxed")
	if err == nil && relaxed {
		setCacheCheckerForBatchCreation(proc)
	}
}
