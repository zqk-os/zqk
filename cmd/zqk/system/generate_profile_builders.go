package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// NewGenerateProfileBuildersCmd creates a command to generate profile builder files from YAML profile files
func NewGenerateProfileBuildersCmd() *cobra.Command {
	var (
		profilesDir string
		outputDir   string
		overwrite   bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate versioned profile builder Go files from YAML profile files",
		"Generate versioned profile builder Go files from YAML profile files.",
		"",
		"This command reads YAML profile files and generates corresponding Go builder",
		"files following the versioned builder pattern. Each builder is versioned at v1_0_0",
		"(semantic versioning format).",
	).
		AddExample("Generate profile builders for all profiles in the default directory", "%s system generate-profile-builders").
		AddExample("Generate profile builders from a specific directory", "%s system generate-profile-builders --profiles-dir "+paths.ProcessInternalProfileSpecsDir).
		AddExample("Generate profile builders to a specific output directory", "%s system generate-profile-builders --output-dir pkg/specbuilder/profile_builders").
		AddExample("Overwrite existing profile builder files", "%s system generate-profile-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-profile-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(systemProfileHuman)
		if ctx != nil && ctx.Profile != emptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		// Default profiles directory
		if profilesDir == emptyValue {
			profilesDir = paths.ProcessInternalProfileSpecsDir
		}

		// Default output directory
		if outputDir == emptyValue {
			outputDir = "pkg/specbuilder/profile_builders"
		}

		// Ensure output directory exists
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		// Find all YAML files
		entries, err := fileutil.ReadDir(profilesDir)
		if err != nil {
			if fileutil.IsNotExist(err) {
				logging.Fluent(logger).Info("profiles directory does not exist, skipping profile builder generation").Log()
				return nil
			}
			return errfmt.Newf("failed to read profiles directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting profile builder generation").Log()
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
			if entry.Name() == "_placeholder.yaml" || objects.IsHashedFilename(entry.Name()) {
				continue
			}

			profilePath := filepath.Join(profilesDir, entry.Name())

			// Check if output file already exists (unless overwrite is set)
			baseName := strings.TrimSuffix(entry.Name(), ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			// Output file is now in sibling directory: ../bldr_profile_v1/{profile_name}_builder.go
			parentDir := filepath.Dir(outputDir)
			outputFile := filepath.Join(parentDir, "bldr_profile_v1", fmt.Sprintf("%s_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", entry.Name())).Log()
					skipped++
					continue
				}
			}

			// Generate builder
			if err := profile_builders.GenerateBuilderFromYAML(profilePath, outputDir); err != nil {
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

	cmd.Flags().StringVar(&profilesDir, "profiles-dir", "", "Directory containing YAML profile files (default: "+paths.ProcessInternalProfileSpecsDir+")")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated builder files (default: pkg/specbuilder/profile_builders)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing builder files")

	cli.AddCommonFlags(cmd)
	return cmd
}
