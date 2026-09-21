package scheduler

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	clictx "github.com/zqk-os/zqk/internal/cli/context"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Convergence spec + spec_ref children under .zqk/cli/specs/scheduler/convergence/.
const convergenceParentSpecRelative = "scheduler/convergence_command.yaml"

// NewSchedulerConvergenceCmd builds scheduler convergence measure/overseer/promotion helpers from
// declarative YAML via CommandSpecBuilder (run_e registry + spec_ref).
func NewSchedulerConvergenceCmd() *cobra.Command {
	cmd, err := buildSchedulerConvergenceCmdFromSpec()
	if err != nil {
		return convergenceSpecLoadFailureCmd(err)
	}
	attachConvergenceCommonFlags(cmd)
	bindConvergenceAsyncProgress(cmd)
	return cmd
}

func convergenceSpecLoadFailureCmd(loadErr error) *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerConvergenceCommandBuilder(), &cobra.Command{
		Use:   "convergence",
		Short: "Convergence measurement (spec load failed)",
		Long:  "Could not load command specs from .zqk/cli/specs; see error when running any subcommand.",
		RunE: func(*cobra.Command, []string) error {
			return errfmt.Newf("scheduler convergence spec load failed").Wrap(loadErr)
		},
	})
}

func buildSchedulerConvergenceCmdFromSpec() (*cobra.Command, error) {
	projectRoot, err := resolveConvergenceSpecsProjectRoot()
	if err != nil {
		return nil, err
	}
	specPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CLISpecsDir, convergenceParentSpecRelative)
	data, err := fileutil.ReadFile(specPath)
	if err != nil {
		return nil, errfmt.Newf("read %s", specPath).Wrap(err)
	}
	var spec clipkg.CommandSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Newf("parse convergence command spec").Wrap(err)
	}
	specsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CLISpecsDir)

	b := clipkg.NewCommandSpecBuilder(&spec).
		WithSpecsDir(specsDir).
		RegisterRunE("schedulerConvergenceMeasure", runTestFailuresConvergenceFromCmd).
		RegisterRunE("schedulerConvergenceOverseer", runTestFailuresConvergenceOverseerFromCmd).
		RegisterRunE("schedulerConvergencePromotionReadiness", runTestFailuresConvergenceFromCmd).
		RegisterRunE("schedulerConvergenceRecordOverseerRun", runTestFailuresConvergenceOverseerFromCmd).
		RegisterRunE("schedulerConvergenceNestSpawn", RunConvergenceNestSpawn).
		RegisterRunE("schedulerConvergenceNestLink", RunConvergenceNestLink).
		RegisterRunE("schedulerConvergenceNestStatus", RunConvergenceNestStatus)

	return b.Build()
}

// resolveConvergenceSpecsProjectRoot finds the checkout that holds .zqk/cli/specs.
// ZQK_TEST_ROOT can redirect ResolveProjectRoot to an empty fixture tree; command DNA still
// lives in the workspace, so fall back to FindWorkspaceRoot when the fixture has no specs.
// fold into a shared ResolveCLISpecsRoot helper.
func resolveConvergenceSpecsProjectRoot() (string, error) {
	candidates := []string{cli.ResolveProjectRoot(".")}
	if ws := clictx.FindWorkspaceRoot("."); ws != "" {
		candidates = append(candidates, ws)
	}
	for _, root := range candidates {
		if root == "" {
			continue
		}
		specPath := filepath.Join(root, paths.ProjectDataDir, paths.CLISpecsDir, convergenceParentSpecRelative)
		if _, err := fileutil.Stat(specPath); err == nil {
			return root, nil
		}
	}
	return "", errfmt.Errorf("project root not found (need .zqk/cli/specs with scheduler convergence DNA)")
}

func attachConvergenceCommonFlags(parent *cobra.Command) {
	for _, sub := range parent.Commands() {
		first := subcommandUseWord(sub)
		switch first {
		case "measure", "overseer", "nest-status":
			cli.AddCommonFlagsExcluding(sub, []string{cli.FlagFormat, cli.FlagVerbose, cli.FlagTimeout, cli.FlagColumns})
		case "promotion-readiness", "record-overseer-run", "nest-spawn", "nest-link":
			cli.AddCommonFlags(sub)
		}
	}
}

func bindConvergenceAsyncProgress(parent *cobra.Command) {
	for _, sub := range parent.Commands() {
		first := subcommandUseWord(sub)
		switch first {
		case "measure":
			cli.BindAsyncProgress(sub, runTestFailuresConvergenceFromCmd)
		case "overseer":
			cli.BindAsyncProgress(sub, runTestFailuresConvergenceOverseerFromCmd)
		}
	}
}

func subcommandUseWord(cmd *cobra.Command) string {
	u := strings.Fields(cmd.Use)
	if len(u) == 0 {
		return ""
	}
	return u[0]
}
