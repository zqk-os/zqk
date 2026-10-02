package zqkdev

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/appledouble"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/lifecycle_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewGenerateLifecycleBuildersCmd creates a command to generate lifecycle builder files from YAML lifecycle files
func NewGenerateLifecycleBuildersCmd() *cobra.Command {
	var (
		lifecyclesDir string
		outputDir     string
		overwrite     bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate lifecycle builder Go files from YAML lifecycle files",
		"Generate lifecycle builder Go files from YAML lifecycle files.",
		"",
		"This command reads YAML lifecycle files and generates corresponding Go builder",
		"files following the versioned builder pattern. Each builder is versioned at v1_0_0",
		"(semantic versioning format).",
	).
		AddExample("Generate lifecycle builders for all lifecycles in the default directory", "%s system generate-lifecycle-builders").
		AddExample("Generate lifecycle builders from a specific directory", "%s system generate-lifecycle-builders --lifecycles-dir "+paths.ProcessInternalLifecyclesDir).
		AddExample("Generate lifecycle builders to a specific output directory", "%s system generate-lifecycle-builders --output-dir pkg/specbuilder/lifecycle_builders").
		AddExample("Overwrite existing lifecycle builder files", "%s system generate-lifecycle-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-lifecycle-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		logger := cmdLogger(cmd)

		// Default lifecycles directory
		if lifecyclesDir == EmptyValue {
			lifecyclesDir = paths.ProcessInternalLifecyclesDir
		}

		// Default output directory
		if outputDir == EmptyValue {
			outputDir = "pkg/specbuilder/lifecycle_builders"
		}

		// Ensure output directory exists
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		parentDir := filepath.Dir(outputDir)
		v1Dir := filepath.Join(parentDir, "bldr_lifecycle_v1")
		if err := fileutil.MkdirAll(v1Dir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create bldr_lifecycle_v1 directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting lifecycle builder generation").Log()
		skipped := 0
		var items []GenerateItem

		err := filepath.WalkDir(lifecyclesDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d == nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if appledouble.SkipNameInReadDir(d.Name()) {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml") {
				return nil
			}
			if d.Name() == "_placeholder.yaml" || objects.IsHashedFilename(d.Name()) {
				return nil
			}

			baseName := strings.TrimSuffix(d.Name(), ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			baseName = strings.TrimSuffix(baseName, "_lifecycle")
			outputFile := filepath.Join(parentDir, "bldr_lifecycle_v1", fmt.Sprintf("%s_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", d.Name())).Log()
					skipped++
					return nil
				}
			}

			items = append(items, GenerateItem{YAMLPath: path, BaseName: baseName})
			return nil
		})
		if err != nil {
			return errfmt.Newf("failed to read lifecycles directory").Wrap(err)
		}

		result := ConcurrentGenerate(cmd.Context(), items, func(_ context.Context, item GenerateItem) error {
			return lifecycle_builders.GenerateBuilderFromYAML(item.YAMLPath, outputDir)
		})

		return logSummaryAndCheckErrors(logger, result.Generated, skipped, result.Errors)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&lifecyclesDir, "lifecycles-dir", "", "Directory containing lifecycle YAML files (default: "+paths.ProcessInternalLifecyclesDir+")")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated Go files (default: pkg/specbuilder/lifecycle_builders)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing lifecycle builder files")

	cli.AddCommonFlags(cmd)
	return cmd
}
