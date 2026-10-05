package zqkdev

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
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
		logger := cmdLogger(cmd)

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

		destDir, _ := instancebuilders.InstanceBuilderOutput(outputDir)
		items, skipped, err := CollectGenerateItems(specsDir, logger, overwrite, func(baseName string, d os.DirEntry) (string, string) {
			outputFile := filepath.Join(destDir, fmt.Sprintf("%s_instance_builder.go", baseName))
			return baseName, outputFile
		})
		if err != nil {
			return errfmt.Newf("failed to read specs directory").Wrap(err)
		}

		// Concurrent generation with bounded errgroup
		result := ConcurrentGenerate(cmd.Context(), items, func(_ context.Context, item GenerateItem) error {
			return instancebuilders.GenerateInstanceBuilderFromSpec(item.YAMLPath, outputDir, "")
		})

		return logSummaryAndCheckErrors(logger, result.Generated, skipped, result.Errors)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	AddBuilderFlags(cmd, &specsDir, "specs-dir", paths.ProcessInternalObjectSpecsDir, "Directory containing YAML spec files", &outputDir, "pkg/specbuilder/instance_builders", &overwrite)
	return cmd
}
