package system

import (
	"fmt"
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
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewGenerateTraitBuildersCmd creates a command to generate trait builder files from YAML trait files
func NewGenerateTraitBuildersCmd() *cobra.Command {
	var (
		traitsDir string
		outputDir string
		overwrite bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate versioned trait builder Go files from YAML trait files",
		"Generate versioned trait builder Go files from YAML trait files.",
		"",
		"This command reads YAML trait files and generates corresponding Go builder",
		"files following the versioned builder pattern. Each builder is versioned at v1_0_0",
		"(semantic versioning format).",
	).
		AddExample("Generate trait builders for all traits in the default directory", "%s system generate-trait-builders").
		AddExample("Generate trait builders from a specific directory", "%s system generate-trait-builders --traits-dir "+paths.ProcessInternalTraitsDir).
		AddExample("Generate trait builders to a specific output directory", "%s system generate-trait-builders --output-dir pkg/specbuilder/trait_builders").
		AddExample("Overwrite existing trait builder files", "%s system generate-trait-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-trait-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(systemProfileHuman)
		if ctx != nil && ctx.Profile != emptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		// Default traits directory
		if traitsDir == emptyValue {
			traitsDir = paths.ProcessInternalTraitsDir
		}

		// Default output directory
		if outputDir == emptyValue {
			outputDir = "pkg/specbuilder/trait_builders"
		}

		// Ensure output directory exists
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		// Find all YAML files
		entries, err := fileutil.ReadDir(traitsDir)
		if err != nil {
			return errfmt.Newf("failed to read traits directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting trait builder generation").Log()
		generated := 0
		skipped := 0
		errors := 0

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
			if objects.IsHashedFilename(entry.Name()) {
				continue
			}

			traitPath := filepath.Join(traitsDir, entry.Name())

			// Check if output file already exists (unless overwrite is set)
			baseName := strings.TrimSuffix(entry.Name(), ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			// Output file is now in sibling directory: ../bldr_trait_v1/{trait_name}_builder.go
			parentDir := filepath.Dir(outputDir)
			outputFile := filepath.Join(parentDir, "bldr_trait_v1", fmt.Sprintf("%s_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", entry.Name())).Log()
					skipped++
					continue
				}
			}

			// Generate builder
			if err := trait_builders.GenerateBuilderFromYAML(traitPath, outputDir); err != nil {
				// Skip non-active traits (expected behavior)
				if strings.Contains(err.Error(), "status") {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (non-active trait)", entry.Name())).Log()
					skipped++
					continue
				}
				logging.Fluent(logger).Error(fmt.Sprintf("Failed to generate builder for %s", entry.Name()), err).Log()
				errors++
				continue
			}

			logging.Fluent(logger).Debug(fmt.Sprintf("Generated builder for %s", entry.Name())).Log()
			generated++
		}

		// Summary (done)
		logging.Fluent(logger).Info(fmt.Sprintf("Summary: Generated %d, Skipped %d, Errors %d", generated, skipped, errors)).Log()

		if errors > 0 {
			return errfmt.Errorf("generation completed with %d errors", errors)
		}

		return nil
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&traitsDir, "traits-dir", "", "Directory containing YAML trait files (default: "+paths.ProcessInternalTraitsDir+")")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated builder files (default: pkg/specbuilder/trait_builders)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing builder files")

	cli.AddCommonFlags(cmd)
	return cmd
}
