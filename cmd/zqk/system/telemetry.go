package system

import (
	"fmt"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/telemetry"
	"github.com/spf13/cobra"
)

func NewTelemetryCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"View telemetry from running agent tasks",
		"View aggregated telemetry logs and metrics.",
		"",
		"This command aggregates and queries telemetry data from all agent tasks.",
	).ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemTelemetryCommandBuilder(), &cobra.Command{
		Use:  "telemetry",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cli.GetContext(cmd)
			profile := profileOrDefault(ctx.Profile, systemProfileHuman)
			logger := logging.GetLoggerFromProfile(profile)

			projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
			if projectRoot == "" {
				return errfmt.Errorf("not a ZQK project (no project root found)")
			}

			factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
			if err != nil {
				return err
			}
			storageProvider := factory.GetStorage()
			if storageProvider != nil {
				defer func() { _ = storageProvider.Shutdown(cmd.Context()) }()
			}

			secCtx := pkgctx.NewSystemSecurityContext()
			storageCtx := pkgctx.NewStorageContext()

			listRes, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{
				Kind: objects.KindAgentTask,
			})
			if err != nil {
				return err
			}

			daemon := telemetry.NewDaemon(logger)
			if err := daemon.Aggregate(cmd.Context()); err != nil {
				return err
			}

			format := cli.GetFormat(cmd)
			switch format {
			case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
				if err := cli.FormatOutput(cmd, listRes.Objects); err != nil {
					return cli.Guard(cmd).Err(err).Return()
				}
				return nil
			default:
				// Output simple table or text
				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Swarm Telemetry: %d active agent tasks tracked.\n", len(listRes.Objects))))
				return nil
			}
		},
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)
	return cmd
}
