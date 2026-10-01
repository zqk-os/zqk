package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/appledouble"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/api_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewGenerateAPIBuildersCmd creates the `system generate-api_spec-builders` command.
//
// Delicate — which directory is scanned:
// Canonical `*_api.yaml` for `pkg/specbuilder/bldr_api_v1` lives under
// `.zqk/specs/api/`. The tree may also contain `*_api.yaml`
// files directly under `.zqk/specs/` with overlapping names but
// different content (duplicates or legacy). The generator only reads one flat
// directory (no recursion). If the default pointed at `_internal` root,
// `make build-all` (which runs this with `--overwrite` and no `--api_specs-dir`)
// would regenerate builders from the wrong inputs and produce large spurious
// diffs. Do not change the default api_specs path casually; if both trees must
// be supported, that requires an explicit design (e.g. documented migration or
// deleting the obsolete copies).
func NewGenerateAPIBuildersCmd() *cobra.Command {
	var (
		api_specsDir string
		outputDir    string
		overwrite    bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate versioned api_spec builder Go files from YAML api_spec files",
		"Generate versioned api_spec builder Go files from YAML api_spec files.",
		"",
		"This command reads YAML api_spec files and generates corresponding Go builder",
		"files following the versioned builder pattern. Each builder is versioned at v1_0_0",
		"(semantic versioning format).",
		"",
		"Default input directory is "+paths.ProcessInternalAPISpecsDir+" (canonical). Do not",
		"point the default at "+paths.ProcessInternalDir+" root: overlapping YAML there can",
		"differ from api_specs/ and will rewrite bldr_api_v1 with the wrong content.",
	).
		AddExample("Generate api_spec builders for all api_specs in the default directory", "%s system generate-api_spec-builders").
		AddExample("Generate from _internal root only (legacy; default is api_specs/)", "%s system generate-api_spec-builders --api-specs-dir "+paths.ProcessInternalDir).
		AddExample("Generate api_spec builders to a specific output directory", "%s system generate-api_spec-builders --output-dir pkg/specbuilder/api_builders").
		AddExample("Overwrite existing api_spec builder files", "%s system generate-api_spec-builders --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-api-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		profile := systemProfileHuman
		if ctx != nil && ctx.Profile != emptyValue {
			profile = ctx.Profile
		}

		loggingCtx := pkgctx.NewLoggingContext(pkgctx.LoggingProfile(profile))
		if cli.IsVerbose(cmd) {
			loggingCtx = loggingCtx.WithSuppressDebugToStdout(false)
		}
		logger := logging.GetLoggerFromLoggingContext(cmd.Context(), loggingCtx)

		// Default api_specs directory — see doc comment on NewGenerateAPIBuildersCmd.
		if api_specsDir == emptyValue {
			api_specsDir = paths.ProcessInternalAPISpecsDir
		}

		// Default output directory
		if outputDir == emptyValue {
			outputDir = "pkg/specbuilder/api_builders"
		}

		// Ensure output directory exists
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		// Find all *_api.yaml files
		entries, err := fileutil.ReadDir(api_specsDir)
		if err != nil {
			if fileutil.IsNotExist(err) {
				logging.Fluent(logger).Info("api_specs directory does not exist, skipping api builder generation").Log()
				return nil
			}
			return errfmt.Newf("failed to read api_specs directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting api_spec builder generation").Log()
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

			// Only process *_api.yaml files
			if !strings.HasSuffix(entry.Name(), "_api.yaml") && !strings.HasSuffix(entry.Name(), "_api_spec.yml") {
				continue
			}

			api_specPath := filepath.Join(api_specsDir, entry.Name())

			// Check if output file already exists (unless overwrite is set)
			baseName := strings.TrimSuffix(entry.Name(), ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")
			// Output file is now in sibling directory: ../bldr_api_v1/{file_name}_builder.go
			parentDir := filepath.Dir(outputDir)
			outputFile := filepath.Join(parentDir, "bldr_api_v1", fmt.Sprintf("%s_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", entry.Name())).Log()
					skipped++
					continue
				}
			}

			// Generate builder
			if err := api_builders.GenerateBuilderFromYAML(api_specPath, outputDir); err != nil {
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

	cmd.Flags().StringVar(&api_specsDir, "api-specs-dir", "", "Directory containing *_api.yaml files (default: "+paths.ProcessInternalAPISpecsDir+")")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated builder files (default: pkg/specbuilder/api_builders)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing builder files")

	cli.AddCommonFlags(cmd)
	return cmd
}
