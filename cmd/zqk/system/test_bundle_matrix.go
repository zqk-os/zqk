package system

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/quality"
	"github.com/spf13/cobra"
)

// NewTestBundleMatrixCmd runs the native test-bundle matrix pipeline (pkg/quality + pkg/pipeline).
// Spec: .zqk/cli/specs/system/test_bundle_matrix_command.yaml
func NewTestBundleMatrixCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemTestBundleMatrixCommandBuilder(), &cobra.Command{Use: "test-bundle-matrix"})
	cli.BindAsyncProgress(cmd, runTestBundleMatrix)
	return cmd
}

func runTestBundleMatrix(cmd *cobra.Command, _ []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	bundlePrefix, _ := cmd.Flags().GetString("bundle-prefix")
	outFlag, _ := cmd.Flags().GetString("matrix-output")
	profile, _ := cmd.Flags().GetString("profile")
	noVerify, _ := cmd.Flags().GetBool("no-verify")
	strict, _ := cmd.Flags().GetBool("strict")
	convergence, _ := cmd.Flags().GetBool("convergence")

	outPath := strings.TrimSpace(outFlag)
	if outPath != "" && !filepath.IsAbs(outPath) {
		outPath = filepath.Join(projectRoot, outPath)
	}

	opts := &quality.TestBundleMatrixOptions{
		ProjectRoot:  projectRoot,
		BundlePrefix: strings.TrimSpace(bundlePrefix),
		OutputPath:   outPath,
		ProfilePath:  strings.TrimSpace(profile),
		SkipVerify:   noVerify,
		StrictVerify: strict,
		Convergence:  convergence,
	}
	return quality.RunTestBundleMatrixPipeline(cmd.Context(), opts)
}
