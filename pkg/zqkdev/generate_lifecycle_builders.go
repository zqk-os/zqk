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
		"Generate versioned lifecycle builder Go files from YAML lifecycle files",
		"Generate versioned lifecycle builder Go files from YAML lifecycle files.",
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
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(SystemProfileHuman)
		if ctx != nil && ctx.Profile != EmptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

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

		// Find all YAML files
		entries, err := fileutil.ReadDir(lifecyclesDir)
		if err != nil {
			return errfmt.Newf("failed to read lifecycles directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting lifecycle builder generation").Log()
		skipped := 0

		var items []GenerateItem
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// Skip macOS AppleDouble/resource-fork files (._*)
			if appledouble.SkipNameInReadDir(entry.Name()) {
				continue
			}

			if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
				continue
			}

			// Skip placeholder files
			if entry.Name() == "_placeholder.yaml" {
				continue
			}

			lifecyclePath := filepath.Join(lifecyclesDir, entry.Name())

			// Check if output file already exists (unless overwrite is set)
			baseName := strings.TrimSuffix(entry.Name(), ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			baseName = strings.TrimSuffix(baseName, "_lifecycle")
			// Output file is now in sibling directory: ../bldr_lifecycle_v1/{object_type}_builder.go
			parentDir := filepath.Dir(outputDir)
			outputFile := filepath.Join(parentDir, "bldr_lifecycle_v1", fmt.Sprintf("%s_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", entry.Name())).Log()
					skipped++
					continue
				}
			}

			items = append(items, GenerateItem{YAMLPath: lifecyclePath, BaseName: baseName})
		}

		result := ConcurrentGenerate(cmd.Context(), items, func(_ context.Context, item GenerateItem) error {
			return lifecycle_builders.GenerateBuilderFromYAML(item.YAMLPath, outputDir)
		})

		// Summary (done)
		logging.Fluent(logger).Info(fmt.Sprintf("Summary: Generated %d, Skipped %d, Errors %d", result.Generated, skipped, result.Errors)).Log()

		if result.Errors > 0 {
			return errfmt.Errorf("generation completed with %d errors", result.Errors)
		}

		return nil
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&lifecyclesDir, "lifecycles-dir", "", "Directory containing YAML lifecycle files (default: "+paths.ProcessInternalLifecyclesDir+")")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated builder files (default: pkg/specbuilder/lifecycle_builders)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing builder files")

	cli.AddCommonFlags(cmd)
	return cmd
}
