package system

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/validation/scenario"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	cmd.MarkFlagRequired("file")
	cli.AddCommonFlags(cmd)

	return cmd
}

func runValidateScenario(cmd *cobra.Command, filePath string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	profile := profileOrDefault(ctx.Profile, systemProfileHuman)
	logger := logging.GetLoggerFromProfile(profile)

	b, err := os.ReadFile(filePath)
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

	result := executor.Run(context.Background(), scen)

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

	cli.WriteOutput(cmd, []byte(outBuilder.String()))

	if !result.Success {
		return errfmt.Errorf("scenario %s failed", scen.Name)
	}

	return nil
}
