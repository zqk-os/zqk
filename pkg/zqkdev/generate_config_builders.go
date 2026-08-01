package zqkdev

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/config_builders"
	"github.com/spf13/cobra"
)

// NewGenerateConfigBuildersCmd creates the `system generate-config-builders` command.
//
// Delicate — which directory is scanned:
// Canonical `*_config.yaml` for `pkg/specbuilder/bldr_config_v1` lives under
// `docs/architecture/_internal/configs/`. The tree may also contain `*_config.yaml`
// files directly under `docs/architecture/_internal/` with overlapping names but
// different content (duplicates or legacy). The generator only reads one flat
// directory (no recursion). If the default pointed at `_internal` root,
// `make build-all` (which runs this with `--overwrite` and no `--configs-dir`)
// would regenerate builders from the wrong inputs and produce large spurious
// diffs. Do not change the default configs path casually; if both trees must
// be supported, that requires an explicit design (e.g. documented migration or
// deleting the obsolete copies).
func NewGenerateConfigBuildersCmd() *cobra.Command {
	var (
		configsDir string
		outputDir  string
		overwrite  bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate versioned config builder Go files from YAML config files",
		"Generate versioned config builder Go files from YAML config files.",
		"",
		"This command reads YAML config files and generates corresponding Go builder",
		"files following the versioned builder pattern. Each builder is versioned at v1_0_0",
		"(semantic versioning format).",
		"",
		"Default input directory is "+paths.ProcessInternalConfigsDir+" (canonical). Do not",
		"point the default at "+paths.ProcessInternalDir+" root: overlapping YAML there can",
		"differ from configs/ and will rewrite bldr_config_v1 with the wrong content.",
	).
		AddExample("Generate config builders for all configs in the default directory", "%s system generate-config-builders").
		AddExample("Generate from _internal root only (legacy; default is configs/)", "%s system generate-config-builders --configs-dir "+paths.ProcessInternalDir).
		AddExample("Generate config builders to a specific output directory", "%s system generate-config-builders --output-dir pkg/specbuilder/config_builders").
		AddExample("Overwrite existing config builder files", "%s system generate-config-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-config-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(SystemProfileHuman)
		if ctx != nil && ctx.Profile != EmptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		// Default configs directory — see doc comment on NewGenerateConfigBuildersCmd.
		if configsDir == EmptyValue {
			configsDir = paths.ProcessInternalConfigsDir
		}

		// Default output directory
		if outputDir == EmptyValue {
			outputDir = "pkg/specbuilder/config_builders"
		}

		// Ensure output directory exists
		if err := os.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		// Find all *_config.yaml files
		entries, err := os.ReadDir(configsDir)
		if err != nil {
			return errfmt.Newf("failed to read configs directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting config builder generation").Log()
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

			// Only process *_config.yaml files
			if !strings.HasSuffix(entry.Name(), "_config.yaml") && !strings.HasSuffix(entry.Name(), "_config.yml") {
				continue
			}

			configPath := filepath.Join(configsDir, entry.Name())

			// Check if output file already exists (unless overwrite is set)
			baseName := strings.TrimSuffix(entry.Name(), ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			// Output file is now in sibling directory: ../bldr_config_v1/{file_name}_builder.go
			parentDir := filepath.Dir(outputDir)
			outputFile := filepath.Join(parentDir, "bldr_config_v1", fmt.Sprintf("%s_builder.go", baseName))

			if !overwrite {
				if _, err := os.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", entry.Name())).Log()
					skipped++
					continue
				}
			}

			items = append(items, GenerateItem{YAMLPath: configPath, BaseName: baseName})
		}

		// Concurrent generation with bounded errgroup
		result := ConcurrentGenerate(cmd.Context(), items, func(_ context.Context, item GenerateItem) error {
			return config_builders.GenerateBuilderFromYAML(item.YAMLPath, outputDir)
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

	cmd.Flags().StringVar(&configsDir, "configs-dir", "", "Directory containing *_config.yaml files (default: "+paths.ProcessInternalConfigsDir+")")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated builder files (default: pkg/specbuilder/config_builders)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing builder files")

	cli.AddCommonFlags(cmd)
	return cmd
}
