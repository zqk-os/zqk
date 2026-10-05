package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation/scenario"
)

// NewValidateScenarioCmd creates a new validate-scenario command
func NewValidateScenarioCmd() *cobra.Command {
	var file string

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Execute a programmatic validation scenario",
		"Execute a sequence of programmatic validation steps defined in a scenario file.",
		"",
		"This command uses the scenario executor to run command, regex, and file existence checks",
		"to natively verify that policies, architectural mandates, and tests are satisfied.",
	).
		AddExample("Run a specific scenario", "%s system validate-scenario --file scenarios/observability_check.yaml").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemValidateScenarioCommandBuilder(), &cobra.Command{
		Use:  "validate-scenario",
		Args: cobra.NoArgs,
	})

	cli.BindAsyncProgress(cmd, func(c *cobra.Command, args []string) error {
		return runValidateScenario(c, file)
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&file, "file", "", "Path to the scenario YAML file")
	_ = cmd.MarkFlagRequired("file")
	cli.AddCommonFlags(cmd)

	return cmd
}

func runValidateScenario(cmd *cobra.Command, filePath string) error {
	_, logger, err := resolveContextAndLogger(cmd, systemProfileHuman)
	if err != nil {
		return err
	}

	b, err := fileutil.ReadFile(filePath)
	if err != nil {
		return errfmt.Errorf("failed to read scenario file %s: %w", filePath, err)
	}

	var scen scenario.Scenario
	if err := yaml.Unmarshal(b, &scen); err != nil {
		return errfmt.Errorf("failed to unmarshal scenario YAML: %w", err)
	}

	executor := scenario.NewExecutor(logger)
	logging.Fluent(logger).Info("Executing validation scenario").
		String("scenario", scen.Name).
		Log()

	result := executor.Run(context.Background(), scen) // Background: request-or-shutdown derived

	format := cli.GetFormat(cmd)
	if format == cli.FormatJSON || format == cli.FormatYAML {
		return cli.FormatOutput(cmd, result)
	}

	var outBuilder strings.Builder
	outBuilder.WriteString(fmt.Sprintf("Scenario: %s\n", result.ScenarioName))
	outBuilder.WriteString(fmt.Sprintf("Success: %v\n", result.Success))
	outBuilder.WriteString(fmt.Sprintf("Duration: %v\n\n", result.TotalTime))

	for _, sr := range result.StepResults {
		status := "PASS"
		if !sr.Success {
			status = "FAIL"
		}
		outBuilder.WriteString(fmt.Sprintf("[%s] Step: %s (%v)\n", status, sr.StepName, sr.Duration))
		if !sr.Success {
			outBuilder.WriteString(fmt.Sprintf("  Error: %s\n", sr.Error))
		}
	}

	_ = cli.WriteOutput(cmd, []byte(outBuilder.String()))

	if !result.Success {
		return errfmt.Errorf("scenario %s failed", scen.Name)
	}

	return nil
}
