package matrix

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// NewMatrixDimensionsCmd creates the matrix dimensions CLI command.
func NewMatrixDimensionsCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("dimensions")
	builder.WithShort("List canonical verification matrix evaluation dimensions and bound kernel policies")
	builder.WithLong(`Displays the authoritative evaluation dimensions (HCODE, EFFPERF, ERRHYG, CONCURR, SECOBS, DOCSIG)
tracked in the Continuous Source Verification Matrix, along with their bound Knowledge Kernel policy IDs.`)
	builder.WithRunE(func(cmd *cobra.Command, _ []string) error {
		dims := matrix.DefaultDimensions()

		cmd.Println("================================================================================")
		cmd.Println("  Continuous Source Verification Matrix: Canonical Evaluation Dimensions")
		cmd.Println("================================================================================")
		for _, d := range dims {
			cmd.Printf("Code     : %s\n", d.Code)
			cmd.Printf("Name     : %s\n", d.Name)
			cmd.Printf("Policy   : %s\n", d.PolicyID)
			cmd.Printf("Severity : %s\n", d.Severity)
			cmd.Printf("Scope    : %s\n", d.Description)
			cmd.Println("--------------------------------------------------------------------------------")
		}
		return nil
	})
	cmd := builder.Build()
	cmd.Example = paths.RewriteCanonicalCLIInvocations(`  zqk matrix dimensions`)
	return cmd
}
