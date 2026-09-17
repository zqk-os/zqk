package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	instancebuilders "github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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
		AddExample("Overwrite existing instance builder files", "%s system generate-instance-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-instance-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(systemProfileHuman)
		if ctx != nil && ctx.Profile != emptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		// Default specs directory
		if specsDir == emptyValue {
			specsDir = paths.ProcessInternalObjectSpecsDir
		}

		// Default output directory
		if outputDir == emptyValue {
			outputDir = "pkg/specbuilder/instance_builders"
		}

		// Ensure output directory exists
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		var generated int32
		skipped := 0
		var errors int32

		// Build items list
		type specItem struct {
			yamlPath  string
			entryName string
		}
		var items []specItem

		err := filepath.WalkDir(specsDir, func(path string, d os.DirEntry, walkErr error) error {
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
			items = append(items, specItem{yamlPath: path, entryName: d.Name()})
			return nil
		})
		if err != nil {
			return errfmt.Newf("failed to read specs directory").Wrap(err)
		}

		var toProcess []specItem
		for _, item := range items {
			// Check if output file already exists (unless overwrite is set)
			baseName := strings.TrimSuffix(item.entryName, ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			// Output file is in sibling directory: ../bldr_instance_v1/{ontology}_instance_builder.go
			parentDir := filepath.Dir(outputDir)
			outputFile := filepath.Join(parentDir, "bldr_instance_v1", fmt.Sprintf("%s_instance_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Info(fmt.Sprintf("Skipping %s (instance builder already exists, use --overwrite to replace)", item.entryName)).Log()
					skipped++
					continue
				}
			}

			toProcess = append(toProcess, item)
		}

		g, _ := errgroup.WithContext(cmd.Context())
		g.SetLimit(8)

		for _, item := range toProcess {
			item := item
			g.Go(func() error {
				// Generate instance builder
				if err := instancebuilders.GenerateInstanceBuilderFromSpec(item.yamlPath, outputDir, ""); err != nil {
					logging.Fluent(logger).Error(fmt.Sprintf("Failed to generate instance builder for %s", item.entryName), err).Log()
					atomic.AddInt32(&errors, 1)
					return nil // do not stop errgroup execution for single-file errors
				}

				logging.Fluent(logger).Info(fmt.Sprintf("Generated instance builder for %s", item.entryName)).Log()
				atomic.AddInt32(&generated, 1)
				return nil
			})
		}

		_ = g.Wait()

		// Summary
		logging.Fluent(logger).Info(fmt.Sprintf("\nSummary: Generated %d, Skipped %d, Errors %d", atomic.LoadInt32(&generated), skipped, atomic.LoadInt32(&errors))).Log()

		if atomic.LoadInt32(&errors) > 0 {
			return errfmt.Errorf("generation completed with %d errors", atomic.LoadInt32(&errors))
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
