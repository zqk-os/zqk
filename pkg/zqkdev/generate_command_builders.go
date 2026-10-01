package zqkdev

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/appledouble"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewGenerateCommandBuildersCmd creates a command to generate command builder files from YAML specs
func NewGenerateCommandBuildersCmd() *cobra.Command {
	var (
		specsDir  string
		outputDir string
		overwrite bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate command builder Go files from YAML command spec files",
		"Generate command builder Go files from YAML command spec files.",
		"",
		"This command reads YAML command spec files and generates corresponding Go builder",
		"files following the versioned builder pattern. Each builder is versioned at v1_0_0",
		"(semantic versioning format).",
		"",
		"The generated builders use the CommandBuilder/CRUDCommandBuilder pattern and",
		"can be used to create cobra.Command instances programmatically.",
	).
		AddExample("Generate builders for all command specs", "%s system generate-command-builders").
		AddExample("Generate builders from a specific directory", "%s system generate-command-builders --specs-dir .zqk/cli/specs").
		AddExample("Generate builders to a specific output directory", "%s system generate-command-builders --output-dir pkg/cli").
		AddExample("Overwrite existing builder files", "%s system generate-command-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-command-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(SystemProfileHuman)
		if ctx != nil && ctx.Profile != EmptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		// Default specs directory
		if specsDir == EmptyValue {
			specsDir = paths.CLICommandSpecsDir
		}

		// Default output directory
		if outputDir == EmptyValue {
			outputDir = "pkg/cli"
		}

		// Ensure output directory exists
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		// Recursively find all YAML files in specs directory
		var yamlFiles []string
		err := filepath.Walk(specsDir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// scheduler/convergence/*.yaml are spec_ref fragments for CommandSpecBuilder only; parent is scheduler/convergence_command.yaml.
			if strings.Contains(path, "/scheduler/convergence/") || strings.Contains(path, "\\scheduler\\convergence\\") {
				return nil
			}
			if appledouble.SkipPathInTreeWalk(path) {
				return nil
			}
			if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
				yamlFiles = append(yamlFiles, path)
			}
			return nil
		})
		if err != nil {
			return errfmt.Newf("failed to walk specs directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting command builder generation").Log()
		skipped := 0

		// Build GenerateItems, filtering skips up front
		var items []GenerateItem
		for _, yamlPath := range yamlFiles {
			relPath, err := filepath.Rel(specsDir, yamlPath)
			if err != nil {
				logging.Fluent(logger).Error(fmt.Sprintf("Failed to get relative path for %s", yamlPath), err).Log()
				continue
			}

			baseName := strings.TrimSuffix(relPath, "_command.yaml")
			baseName = strings.TrimSuffix(baseName, "_command.yml")
			baseName = strings.TrimSuffix(baseName, ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			baseName = strings.ReplaceAll(baseName, string(filepath.Separator), "_")
			baseName = strings.ReplaceAll(baseName, "/", "_")

			outputFile := filepath.Join(outputDir, "bldr_cli_cmd_v1", fmt.Sprintf("%s_command_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", relPath)).Log()
					skipped++
					continue
				}
			}

			items = append(items, GenerateItem{YAMLPath: yamlPath, BaseName: baseName})
		}

		// Concurrent generation with bounded errgroup
		result := ConcurrentGenerate(cmd.Context(), items, func(_ context.Context, item GenerateItem) error {
			return clipkg.GenerateCommandBuilderFromYAML(item.YAMLPath, outputDir)
		})

		// Summary
		logging.Fluent(logger).Info(fmt.Sprintf("Summary: Generated %d, Skipped %d, Errors %d", result.Generated, skipped, result.Errors)).Log()

		if result.Errors > 0 {
			return errfmt.Errorf("generation completed with %d errors", result.Errors)
		}

		return nil
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&specsDir, "specs-dir", "", "Directory containing YAML command spec files (default: .zqk/cli/specs)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated builder files (default: pkg/cli)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing command builder files")

	cli.AddCommonFlags(cmd)
	return cmd
}
