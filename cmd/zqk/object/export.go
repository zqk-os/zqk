package object

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewExportCmd creates the object export command
func NewExportCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectExportCommandBuilder(), &cobra.Command{
		Use:   "export [kind]",
		Short: "Export objects to YAML or JSON",
		Long: fmt.Sprintf(`Export objects matching filters to a file or stdout.

Examples:
  # Export all backlog_item to YAML file
  %s object export backlog_item --out items.yaml
  # Export with filter and JSON format
  %s object export backlog_item --filter status=planned --format json --out items.json
  # Write to stdout
  %s object export backlog_item --out -`,
			paths.CLICommandName, paths.CLICommandName, paths.CLICommandName),
		Args: cobra.MaximumNArgs(1),
		RunE: runExport,
	})
	cli.AddCommonFlags(cmd)
	clipkg.AddQueryFlags(cmd)

	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidatePositional0Opt

	return cmd
}

func runExport(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		kind := ""
		if len(args) > 0 {
			var ok bool
			kind, ok = kindCanonicalFromPRERun(cmd)
			if !ok {
				var rerr error
				kind, rerr = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), args[0])
				if rerr != nil {
					return cli.Guard(cmd).Err(rerr).Return()
				}
			}
		}
		if kind == emptyValue {
			return cli.Guard(cmd).Require(false, "kind is required (e.g. backlog_item)").Return()
		}

		flags, _, err := parseListFlags(cmd, proc)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}
		storageCtx := pkgctx.NewStorageContext()
		listFilter := buildListFilter(kind, flags, storageCtx)

		// Use context format (yaml/json from --format or profile); default yaml for export
		formatStr := string(proc.Format())
		if formatStr == emptyValue || formatStr == objectFormatTable {
			formatStr = objectFormatYAML
		}
		format := storage.ExportFormat(formatStr)

		data, err := storage.ExportObjects(
			proc.OperationContext(),
			proc.Storage(),
			proc.SecurityContext(),
			storageCtx,
			listFilter,
			format,
		)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Export failed", err).Log()
			return cli.Guard(cmd).Err(err).Wrapf("export failed: %w").Return()
		}

		outputPath, _ := cmd.Flags().GetString("out")
		if outputPath == emptyValue || outputPath == "-" {
			return cli.WriteOutput(cmd, data)
		}
		if err := fileutil.WriteFile(outputPath, data, paths.FilePerm600); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to write output file", err).
				String("output", outputPath).
				Log()
			return cli.EnhanceError(cmd, errfmt.Errorf("failed to write %s: %w", outputPath, err))
		}
		logging.FluentEvent(proc.Logger()).Info("Exported objects").
			String("output", outputPath).
			Int("bytes", len(data)).
			Log()
		return nil
	})(cmd, args)
}
