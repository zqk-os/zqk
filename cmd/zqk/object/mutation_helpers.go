package object

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
		if val, err := cmd.Flags().GetString("reason-code"); err == nil {
			reasonCode = val
		}
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

// readRequiredFileFlag reads the content of the file specified by the --file flag.
func readRequiredFileFlag(cmd *cobra.Command) (string, []byte, error) {
	filePath, err := cmd.Flags().GetString("file")
	if err != nil {
		return "", nil, cli.Guard(cmd).Err(err).Return()
	}
	if filePath == emptyValue {
		return "", nil, cli.Guard(cmd).Require(false, "--file is required").Return()
	}
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return filePath, nil, cli.Guard(cmd).Err(err).Wrapf("failed to read file: %w").Return()
	}
	return filePath, data, nil
}

// resolveSingleObjectID resolves natural language intents or positional ID into a canonical ID.
func resolveSingleObjectID(cmd *cobra.Command, proc *cli.Processor, rawIDs []string) (string, error) {
	idArg := rawIDs[0]
	id, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", idArg)
	if err != nil {
		return "", cli.Guard(cmd).Err(err).Wrapf("semantic routing failed").Return()
	}
	return id, nil
}

// handleDryRunAndRelaxed executes a dry-run check, returning true if handled, otherwise configures relaxed mode.
func handleDryRunAndRelaxed(cmd *cobra.Command, proc *cli.Processor, runDryRun func() (bool, error)) (bool, error) {
	handled, err := runDryRun()
	if err != nil {
		return false, cli.Guard(cmd).Err(err).Return()
	}
	if handled {
		return true, nil
	}
	configureRelaxedMode(cmd, proc)
	return false, nil
}

// prepareCreateAndDryRun validates kind matches and handles dry run. Returns true if handled.
func prepareCreateAndDryRun(cmd *cobra.Command, proc *cli.Processor, kind string, objData map[string]any) (bool, error) {
	if err := ensureKindMatches(objData, kind, proc); err != nil {
		return false, cli.Guard(cmd).Err(err).Return()
	}
	return handleDryRunAndRelaxed(cmd, proc, func() (bool, error) {
		return handleDryRun(cmd, objData, kind, proc)
	})
}
