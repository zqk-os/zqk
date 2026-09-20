package object

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewRenameCmd creates the object rename command (BLI-851).
// Uses generated builder from .zqk/cli/specs/object/rename_command.yaml.
func NewRenameCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectRenameCommandBuilder()
	cmd.RunE = runRename
	return cmd
}

func runRename(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		oldID := args[0]
		newID := args[1]

		var err error
		_ = err

		updateReferences, _ := cmd.Flags().GetBool("update-references")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		logging.FluentEvent(proc.Logger()).Debug("Renaming object").
			String("old_id", oldID).
			String("new_id", newID).
			Bool("update_references", updateReferences).
			Bool("dry_run", dryRun).
			Log()

		if dryRun {
			obj, err := proc.Storage().Read(proc.WithCLIOperation(), proc.SecurityContext(), oldID)
			if err != nil {
				logging.FluentEvent(proc.Logger()).Error("Failed to read object for dry-run", err).
					String("id", oldID).
					Log()
				return cli.Guard(cmd).Err(err).Wrapf("failed to read object: %w").Return()
			}
			kind, _ := obj[objects.FieldKeyKind].(string)
			var buf bytes.Buffer
			fmt.Fprintf(&buf, "Would rename object %s to %s\n", oldID, newID)
			fmt.Fprintf(&buf, "  Kind: %s\n", kind)
			fmt.Fprintf(&buf, "  Update references: %v\n", updateReferences)
			return cli.WriteOutput(cmd, buf.Bytes())
		}

		cliCtx := proc.WithCLIOperation()
		if err := proc.Storage().Rename(cliCtx, proc.SecurityContext(), oldID, newID, updateReferences); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to rename object", err).
				String("old_id", oldID).
				String("new_id", newID).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to rename object: %w").Return()
		}

		logging.FluentEvent(proc.Logger()).Info("Object renamed successfully").
			String("old_id", oldID).
			String("new_id", newID).
			Log()

		msg := fmt.Sprintf("Object %s renamed to %s\n", oldID, newID)
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}
