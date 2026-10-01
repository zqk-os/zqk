package object

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewEvomanShowCmd creates the evoman show subcommand.
// Generated spec: .zqk/cli/specs/object/evoman/show_command.yaml. RunE and BLI-642 filtering in this package.
func NewEvomanShowCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectEvomanShowCommandBuilder()
	cli.BindAsyncProgress(cmd, runEvomanShow)
	return cmd
}

func runEvomanShow(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		id := ""
		if len(args) > 0 {
			id = args[0]
		}

		if id == emptyValue {
			secCtx := pkgctx.NewSystemSecurityContext()
			storageCtx := proc.StorageContext()
			filter := storage.ListFilter{
				Kind:    objects.KindEvolutionManagement,
				Filters: map[string]any{},
				SortBy:  objects.FieldKeyID,
				SortAsc: true,
				Limit:   1,
			}
			result, err := proc.Storage().List(cmd.Context(), secCtx, storageCtx, filter)
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("failed to list evolution management: %w").Return()
			}
			if len(result.Objects) == 0 {
				logging.FluentEvent(proc.Logger()).Info("No evolution management objects found").Log()
				return cli.Guard(cmd).Err(errfmt.Errorf("no evolution management objects found")).Return()
			}
			id, _ = result.Objects[0][objects.FieldKeyID].(string)
			if id == emptyValue {
				return cli.Guard(cmd).Err(errfmt.Errorf("evolution management object missing id")).Return()
			}
		}

		logging.FluentEvent(proc.Logger()).Debug("Reading evolution management").
			ObjectID(id).
			Log()
		obj, err := proc.Storage().Read(cmd.Context(), proc.SecurityContext(), id)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to read evolution management", err).
				ObjectID(id).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to read evolution management: %w").Return()
		}

		if proc.ProjectRoot() != emptyValue && !skipObjectGetFieldACL(proc.SecurityContext()) {
			specsDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalObjectSpecsDir)
			loader := objects.NewSpecLoader(specsDir)
			sac := mcp.NewSpecAccessControlFromObjectsLoader(loader)
			if sac != nil {
				obj = sac.FilterObjectFields(obj, proc.SecurityContext(), "read")
			}
		}

		logging.FluentEvent(proc.Logger()).Debug("Evolution management read successfully").
			ObjectID(id).
			Log()
		return cli.FormatOutput(cmd, obj)
	})(cmd, args)
}
