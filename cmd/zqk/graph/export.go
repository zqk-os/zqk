package graph

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/databook"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewExportCmd creates a new graph export command.
func NewExportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewGraphExportCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		kind, _ := cmd.Flags().GetString("kind")
		outputPath, _ := cmd.Flags().GetString("output")
		return runExport(cmd, format, kind, outputPath)
	}
	return cmd
}

func runExport(cmd *cobra.Command, format, kind, outputPath string) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}

	storageProvider := proc.Storage()
	if storageProvider == nil {
		return errfmt.Errorf("storage provider unavailable")
	}

	ctx := cmd.Context()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	filter := storage.ListFilter{}
	if kind != "" {
		filter.Kind = kind
	}

	res, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return errfmt.Newf("failed to list objects for graph export").Wrap(err)
	}

	db, err := databook.BuildDataBookFromObjects(res.Objects, "zqk-os")
	if err != nil {
		return errfmt.Newf("failed to construct W3C Holon DataBook").Wrap(err)
	}

	var outputBytes []byte
	switch format {
	case "turtle", "ttl", "rdf":
		outputBytes = []byte(databook.ExportToTurtle(db))
	case "databook", "json-ld", "json", "table", "":
		outputBytes, err = databook.ExportToJSONLD(db)
		if err != nil {
			return errfmt.Newf("failed to serialize JSON-LD DataBook").Wrap(err)
		}
	default:
		return errfmt.Errorf("unsupported export format %q; valid formats: databook, turtle, json-ld", format)
	}

	if outputPath != "" {
		if err := fileutil.WriteFile(outputPath, outputBytes, paths.FilePerm644); err != nil {
			return errfmt.Newf("failed to write export to %s", outputPath).Wrap(err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Exported %d holons to %s (format: %s)\n", len(db.Holons), outputPath, format)
		return nil
	}

	return cli.WriteOutput(cmd, outputBytes)
}
