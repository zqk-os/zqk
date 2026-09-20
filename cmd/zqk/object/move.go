package object

import (
	"bytes"
	"fmt"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewMoveCmd creates a new move command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewMoveCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectMoveCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runMove)

	// Mark kind flag as required (not handled by codegen yet)
	//nolint:errcheck // Flag requirement check - error would be caught at runtime
	_ = cmd.MarkFlagRequired("kind")

	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidateFlagKind

	return cmd
}

func runMove(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		id := args[0]

		var err error
		_ = err

		// Get flags
		newKind, err := cmd.Flags().GetString("kind")
		if err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("failed to get kind flag: %w").Return()
		}
		if newKind == emptyValue {
			return cli.Guard(cmd).Require(false, "--kind is required").Return()
		}

		if nk, ok := kindCanonicalFromPRERun(cmd); ok {
			newKind = nk
		} else {
			newKind, err = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), newKind)
			if err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
		}

		updateReferences, err := cmd.Flags().GetBool("update-references")
		if err != nil {
			updateReferences = false
		}

		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			dryRun = false
		}

		logging.FluentEvent(proc.Logger()).Debug("Moving object").
			ObjectID(id).
			String("new_kind", newKind).
			Bool("update_references", updateReferences).
			Bool("dry_run", dryRun).
			Log()

		// Check for dry-run
		if dryRun {
			logging.FluentEvent(proc.Logger()).Info("Dry-run mode: showing what would be moved").
				ObjectID(id).
				Log()
			// Read object to show what would be moved
			obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
			if err != nil {
				logging.FluentEvent(proc.Logger()).Error("Failed to read object for dry-run", err).
					ObjectID(id).
					Log()
				return cli.Guard(cmd).Err(err).Wrapf("failed to read object: %w").Return()
			}

			oldKind, _ := obj[objects.FieldKeyKind].(string)
			logging.FluentEvent(proc.Logger()).Info("Would move object").
				ObjectID(id).
				String("old_kind", oldKind).
				String("new_kind", newKind).
				Bool("update_references", updateReferences).
				Log()

			// Output what would be moved
			var buf bytes.Buffer
			fmt.Fprintf(&buf, "Would move object %s:\n", id)
			fmt.Fprintf(&buf, "  From kind: %s\n", oldKind)
			fmt.Fprintf(&buf, "  To kind: %s\n", newKind)
			fmt.Fprintf(&buf, "  Update references: %v\n", updateReferences)
			return cli.WriteOutput(cmd, buf.Bytes())
		}

		// Perform move
		if err := proc.Storage().Move(proc.WithCLIOperation(), proc.SecurityContext(), id, newKind, updateReferences); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to move object", err).
				ObjectID(id).
				String("new_kind", newKind).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to move object: %w").Return()
		}

		logging.FluentEvent(proc.Logger()).Info("Object moved successfully").
			ObjectID(id).
			String("new_kind", newKind).
			Log()

		msg := fmt.Sprintf("Object %s moved successfully to kind %s\n", id, newKind)
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}
