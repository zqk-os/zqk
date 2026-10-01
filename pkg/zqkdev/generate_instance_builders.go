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
	instancebuilders "github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewGenerateInstanceBuildersCmd creates a command to generate instance builder files from YAML spec files.
// TODO(arch-cli-split): this codegen command belongs in a dedicated developer/tooling
// CLI (e.g. zqk-dev) rather than the core product CLI. When the CLI split is
// implemented, move this under the dev binary or a dev-only command group so
// end users are not exposed to low-level code generation operations.
func NewGenerateInstanceBuildersCmd() *cobra.Command {
	var (
		specsDir  string
		outputDir string
		overwrite bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate instance builder Go files from YAML spec files",
		"Generate instance builder Go files from YAML spec files.",
		"",
		"This command reads YAML object spec files and generates corresponding Go instance builder",
		"files. Each builder handles ALL instances of that object type (not one per instance).",
	).
		AddExample("Generate instance builders for all specs in the default directory", "%s system generate-instance-builders").
		AddExample("Generate instance builders from a specific directory", "%s system generate-instance-builders --specs-dir "+paths.ProcessInternalObjectSpecsDir).
		AddExample("Generate instance builders to a specific output directory", "%s system generate-instance-builders --output-dir pkg/specbuilder/instance_builders").
		AddExample("A pack passes its own instance_builders directory; files are written beside it", "%s system generate-instance-builders --output-dir packs/work/instance_builders").
		AddExample("Overwrite existing instance builder files", "%s system generate-instance-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-instance-builders",
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
			specsDir = paths.ProcessInternalObjectSpecsDir
		}

		// Default output directory
		if outputDir == EmptyValue {
			outputDir = instancebuilders.DefaultInstanceBuilderToolDir
		}

		// Ensure output directory exists
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		skipped := 0
		var items []GenerateItem

		err := filepath.WalkDir(specsDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d == nil {
				return nil //nolint:nilerr // skip unreadable entries
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
			destDir, _ := instancebuilders.InstanceBuilderOutput(outputDir)
			outputFile := filepath.Join(destDir, fmt.Sprintf("%s_instance_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Info(fmt.Sprintf("Skipping %s (instance builder already exists, use --overwrite to replace)", d.Name())).Log()
					skipped++
					return nil
				}
			}

			items = append(items, GenerateItem{YAMLPath: path, BaseName: baseName})
			return nil
		})
		if err != nil {
			return errfmt.Newf("failed to read specs directory").Wrap(err)
		}

		// Concurrent generation with bounded errgroup
		result := ConcurrentGenerate(cmd.Context(), items, func(_ context.Context, item GenerateItem) error {
			return instancebuilders.GenerateInstanceBuilderFromSpec(item.YAMLPath, outputDir, "")
		})

		// Summary
		logging.Fluent(logger).Info(fmt.Sprintf("\nSummary: Generated %d, Skipped %d, Errors %d", result.Generated, skipped, result.Errors)).Log()

		if result.Errors > 0 {
			return errfmt.Errorf("generation completed with %d errors", result.Errors)
		}

		return nil
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&specsDir, "specs-dir", "", "Directory containing YAML spec files (default: "+paths.ProcessInternalObjectSpecsDir+")")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated builder files (default: pkg/specbuilder/instance_builders)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing instance builder files")

	cli.AddCommonFlags(cmd)
	return cmd
}
