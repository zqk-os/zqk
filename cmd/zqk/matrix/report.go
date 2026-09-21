package matrix

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/quality"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func runMatrixReport(cmd *cobra.Command, _ []string) error {
	ctx := cli.GetContext(cmd)
	projectRoot := ""
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == "" {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == "" {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("project root not found; run from repo root or zqk use"))
	}

	name, _ := cmd.Flags().GetString("name")
	registryFlag, _ := cmd.Flags().GetString("registry")
	matrixPath, _ := cmd.Flags().GetString("matrix")
	profilePath, _ := cmd.Flags().GetString("profile")
	goOnly, _ := cmd.Flags().GetBool("go-only")

	resolved, err := quality.ResolveMatrixForCLI(projectRoot, name, registryFlag, matrixPath, profilePath)
	if err != nil {
		if errors.Is(err, quality.ErrNoMatricesConfigured) || errors.Is(err, quality.ErrNoDefaultMatrix) {
			cmd.PrintErrf("Warning: %v\n", err)
			sum := &quality.MatrixReportSummary{
				MatrixName:  name,
				GateColumns: []string{},
				PerGate:     map[string]quality.GateColumnCounts{},
			}
			return cli.FormatOutput(cmd, sum)
		}
		return err
	}

	sum, err := quality.SummarizeMatrixFromPaths(resolved.CSVPath, resolved.ProfilePath, goOnly, resolved.SessionRefColumn)
	if err != nil {
		if os.IsNotExist(err) || fileutil.IsNotExist(err) {
			cmd.PrintErrf("Warning: matrix file missing (%v). Returning empty summary.\n", err)
			sum := &quality.MatrixReportSummary{
				MatrixName:    resolved.Alias,
				CSVPath:       resolved.CSVPath,
				ProfilePath:   resolved.ProfilePath,
				GoOnly:        goOnly,
				RowTotal:      0,
				FullyDoneRows: 0,
				GateColumns:   []string{},
				PerGate:       map[string]quality.GateColumnCounts{},
			}
			return cli.FormatOutput(cmd, sum)
		}
		return err
	}
	sum.MatrixName = resolved.Alias

	return cli.FormatOutput(cmd, sum)
}
