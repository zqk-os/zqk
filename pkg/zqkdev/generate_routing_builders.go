package zqkdev

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/appledouble"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/routing_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewGenerateRoutingBuildersCmd creates a command to generate routing rule builder files from YAML routing rule files
func NewGenerateRoutingBuildersCmd() *cobra.Command {
	var (
		rulesDir  string
		outputDir string
		overwrite bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate versioned routing rule builder Go files from YAML routing rule files",
		"Generate versioned routing rule builder Go files from YAML routing rule files.",
		"",
		"This command reads YAML routing rule files and generates corresponding Go builder",
		"files following the versioned builder pattern. Each builder is versioned at v1_0_0",
		"(semantic versioning format).",
	).
		AddExample("Generate routing rule builders for all rules in the default directory", "%s system generate-routing-builders").
		AddExample("Generate routing rule builders from a specific directory", "%s system generate-routing-builders --rules-dir "+filepath.Join(paths.ProcessInternalDir, "routing_rules")).
		AddExample("Generate routing rule builders to a specific output directory", "%s system generate-routing-builders --output-dir pkg/specbuilder/routing_builders").
		AddExample("Overwrite existing routing rule builder files", "%s system generate-routing-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-routing-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		logger := cmdLogger(cmd)

		// Default rules directory
		if rulesDir == EmptyValue {
			rulesDir = filepath.Join(paths.ProcessInternalDir, "routing_rules")
		}

		// Default output directory
		if outputDir == EmptyValue {
			outputDir = "pkg/specbuilder/routing_builders"
		}

		// Ensure output directory exists
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		// Find all YAML files
		entries, err := fileutil.ReadDir(rulesDir)
		if err != nil {
			return errfmt.Newf("failed to read routing rules directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting routing builder generation").Log()
		skipped := 0

		// Build GenerateItems, filtering skips up front
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
			if objects.IsHashedFilename(entry.Name()) {
				continue
			}

			rulePath := filepath.Join(rulesDir, entry.Name())

			// Check if output file already exists (unless overwrite is set)
			baseName := strings.TrimSuffix(entry.Name(), ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			// Output file is now in sibling directory: ../bldr_routing_v1/{file_name}_builder.go
			parentDir := filepath.Dir(outputDir)
			outputFile := filepath.Join(parentDir, "bldr_routing_v1", fmt.Sprintf("%s_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", entry.Name())).Log()
					skipped++
					continue
				}
			}

			items = append(items, GenerateItem{YAMLPath: rulePath, BaseName: baseName})
		}

		// Concurrent generation with bounded errgroup
		result := ConcurrentGenerate(cmd.Context(), items, func(_ context.Context, item GenerateItem) error {
			return routing_builders.GenerateBuilderFromYAML(item.YAMLPath, outputDir)
		})

		return logSummaryAndCheckErrors(logger, result.Generated, skipped, result.Errors)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	AddBuilderFlags(cmd, &rulesDir, "rules-dir", filepath.Join(paths.ProcessInternalDir, "routing_rules"), "Directory containing YAML routing rule files", &outputDir, "pkg/specbuilder/routing_builders", &overwrite)
	return cmd
}
