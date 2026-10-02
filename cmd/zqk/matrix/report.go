package matrix

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/quality"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func runMatrixReport(cmd *cobra.Command, _ []string) error {
	_, resolved, err := resolveMatrixFromCmd(cmd)
	if err != nil {
		if errors.Is(err, quality.ErrNoMatricesConfigured) || errors.Is(err, quality.ErrNoDefaultMatrix) {
			name, _ := cmd.Flags().GetString("name")
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

	goOnly, _ := cmd.Flags().GetBool("go-only")

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
