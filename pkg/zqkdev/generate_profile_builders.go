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
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/profile_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
		logger := cmdLogger(cmd)

		// Default profiles directory
		if profilesDir == EmptyValue {
			profilesDir = paths.ProcessInternalProfileSpecsDir
		}

		// Default output directory
		if outputDir == EmptyValue {
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

			items = append(items, GenerateItem{YAMLPath: profilePath, BaseName: baseName})
		}

		result := ConcurrentGenerate(cmd.Context(), items, func(_ context.Context, item GenerateItem) error {
			return profile_builders.GenerateBuilderFromYAML(item.YAMLPath, outputDir)
		})

		return logSummaryAndCheckErrors(logger, result.Generated, skipped, result.Errors)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	AddBuilderFlags(cmd, &profilesDir, "profiles-dir", paths.ProcessInternalProfileSpecsDir, "Directory containing YAML profile files", &outputDir, "pkg/specbuilder/profile_builders", &overwrite)
	return cmd
}
