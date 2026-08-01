package object

import (
	"path/filepath"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewSPlanShowCmd creates the splan show subcommand.
// Generated spec: docs/process/command_specs/object/splan/show_command.yaml. RunE and ITEM-642 filtering in this package.
func NewSPlanShowCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectSplanShowCommandBuilder()
	cli.BindAsyncProgress(cmd, runSPlanShow)
	return cmd
}

func runSPlanShow(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		id := ""
		if len(args) > 0 {
			id = args[0]
		}

		if id == emptyValue {
			// List strategic_plan, take first by id
			secCtx := pkgctx.NewSystemSecurityContext()
			storageCtx := proc.StorageContext()
			filter := storage.ListFilter{
				Kind:    objects.KindStrategicPlan,
				Filters: map[string]any{},
				SortBy:  objects.FieldKeyID,
				SortAsc: true,
				Limit:   1,
			}
			result, err := proc.Storage().List(cmd.Context(), secCtx, storageCtx, filter)
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("failed to list strategic plans: %w").Return()
			}
			if len(result.Objects) == 0 {
				logging.FluentEvent(proc.Logger()).Info("No strategic plans found").Log()
				return cli.Guard(cmd).Err(errfmt.Errorf("no strategic plans found")).Return()
			}
			id, _ = result.Objects[0][objects.FieldKeyID].(string)
			if id == emptyValue {
				return cli.Guard(cmd).Err(errfmt.Errorf("strategic plan object missing id")).Return()
			}
		}

		logging.FluentEvent(proc.Logger()).Debug("Reading strategic plan").
			ObjectID(id).
			Log()
		obj, err := proc.Storage().Read(cmd.Context(), proc.SecurityContext(), id)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to read strategic plan", err).
				ObjectID(id).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to read strategic plan: %w").Return()
		}

		// ITEM-642: field-level permissions — filter to fields the security context can read (same as object get)
		if proc.ProjectRoot() != emptyValue {
			specsDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalObjectSpecsDir)
			loader := objects.NewSpecLoader(specsDir)
			sac := mcp.NewSpecAccessControlFromObjectsLoader(loader)
			if sac != nil {
				obj = sac.FilterObjectFields(obj, proc.SecurityContext(), "read")
			}
		}

		logging.FluentEvent(proc.Logger()).Debug("Strategic plan read successfully").
			ObjectID(id).
			Log()
		return cli.FormatOutput(cmd, obj)
	})(cmd, args)
}
