package matrix

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/quality"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
	"github.com/zqk-os/zqk/pkg/verification/matrix/scanners"
)

func cmdContext(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

func defaultMatrixRegistry() *matrix.CheckRegistry {
	registry := matrix.NewCheckRegistry()
	scanner := scanners.NewHardcodedLogicScanner()
	registry.Register(matrix.CheckDefinition{
		ID:            "hardcoded-logic",
		Name:          "Hardcoded Logic Scanner",
		Description:   "Programmatic detection of raw permissions, release tags, and hardcoded literals",
		Kind:          matrix.CheckKindProgrammatic,
		TargetClasses: []matrix.FileClass{matrix.ClassSource, matrix.ClassTest, matrix.ClassScript, matrix.ClassConfigFile},
		Runner:        scanner,
	})
	return registry
}

func initMatrixEngine(projectRoot string, registry *matrix.CheckRegistry) (*matrix.Engine, error) {
	ledgerPath := filepath.Join(projectRoot, paths.ProjectDataDir, "verification_matrix.json")
	engine, err := matrix.NewEngine(projectRoot, ledgerPath, registry)
	if err != nil {
		return nil, errfmt.Errorf("failed to initialize verification matrix: %w", err)
	}
	return engine, nil
}

func setupMatrixCmd(cmd *cobra.Command, registry *matrix.CheckRegistry) (context.Context, string, *matrix.Engine, error) {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return nil, "", nil, err
	}
	engine, err := initMatrixEngine(projectRoot, registry)
	if err != nil {
		return nil, "", nil, err
	}
	return cmdContext(cmd), projectRoot, engine, nil
}

func persistLedger(cmd *cobra.Command, engine *matrix.Engine) error {
	if err := engine.SaveLedger(); err != nil {
		if cmd != nil {
			cmd.PrintErrf("Warning: failed to persist matrix ledger: %v\n", err)
		}
		return errfmt.Errorf("failed to persist matrix ledger: %w", err)
	}
	return nil
}

func resolveProjectRoot(cmd *cobra.Command) (string, error) {
	ctx := cli.GetContext(cmd)
	projectRoot := ""
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == "" && cmd != nil && cmd.Context() != nil {
		projectRoot = pkgctx.GetLifecycleProjectRoot(cmd.Context())
	}
	if projectRoot == "" {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == "" {
		return "", errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("project root not found; run from repo root or zqk use"))
	}
	return projectRoot, nil
}

func resolveMatrixFromCmd(cmd *cobra.Command) (string, quality.MatrixResolution, error) {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return "", quality.MatrixResolution{}, err
	}
	name, _ := cmd.Flags().GetString("name")
	registryFlag, _ := cmd.Flags().GetString("registry")
	matrixPath, _ := cmd.Flags().GetString("matrix")
	profilePath, _ := cmd.Flags().GetString("profile")
	resolved, err := quality.ResolveMatrixForCLI(projectRoot, name, registryFlag, matrixPath, profilePath)
	if err != nil {
		return projectRoot, quality.MatrixResolution{}, err
	}
	return projectRoot, resolved, nil
}

// NewMatrixCmd is the top-level matrix command group (traceability / vetting CSVs).
// See docs/architecture/MATRIX_CLI_STRATEGY.md
func NewMatrixCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Traceability matrices (CSV registries, validation, reports)",
		"Work with named matrices defined in "+filepath.Join(paths.DocsQualityDir, "matrix_registry.yaml")+": reports, row updates (gate columns),",
		"and validation. If the registry file is missing, a default template is initialized automatically.",
	).
		AddExample("Summarize default matrix report", "%s matrix report").
		AddExample("Report for a specific matrix", "%s matrix report --name <matrix_name>").
		AddExample("Update row status", "%s matrix update --file-path pkg/x.go --set status=completed").
		AddExample("List pending rows", "%s matrix get --filter status=pending").
		AddExample("List configured matrix names", "%s matrix list").
		AddExample("Validate matrix CSVs and profiles", "%s matrix validate")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMatrixCommandBuilder(), &cobra.Command{
		Use: "matrix",
	})
	helpBuilder.ApplyToCommand(cmd)
	cmd.AddCommand(NewMatrixReportCmd())
	cmd.AddCommand(NewMatrixListCmd())
	cmd.AddCommand(NewMatrixGetCmd())
	cmd.AddCommand(NewMatrixUpdateCmd())
	cmd.AddCommand(NewMatrixValidateCmd())
	cmd.AddCommand(NewMatrixVerifyCmd())
	cmd.AddCommand(NewMatrixEvaluateCmd())
	cmd.AddCommand(NewMatrixStampCmd())
	cmd.AddCommand(NewMatrixDimensionsCmd())
	cmd.AddCommand(NewMatrixInventoryCmd())
	return cmd
}

// NewMatrixReportCmd wires the generated spec builder to RunE.
func NewMatrixReportCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixReportCommandBuilder()
	c.RunE = runMatrixReport
	return c
}

// NewMatrixListCmd wires the generated list spec builder to RunE.
func NewMatrixListCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixListCommandBuilder()
	c.RunE = runMatrixList
	return c
}

// NewMatrixGetCmd wires the generated get spec builder to RunE.
func NewMatrixGetCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixGetCommandBuilder()
	cli.BindAsyncProgress(c, runMatrixGet)
	return c
}

// NewMatrixUpdateCmd wires the generated update spec builder to RunE.
func NewMatrixUpdateCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixUpdateCommandBuilder()
	cli.BindAsyncProgress(c, runMatrixUpdate)
	return c
}
