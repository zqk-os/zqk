package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const defaultCLISpecsDirRel = ".zqk/cli/specs"

// NewGenerateCommandBuildersCmd creates a command to generate command builder files from YAML specs
func NewGenerateCommandBuildersCmd() *cobra.Command {
	var (
		specsDir                   string
		outputDir                  string
		overwrite                  bool
		includeProcessCommandSpecs bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate command builder Go files from YAML command spec files",
		"Generate command builder Go files from YAML command spec files.",
		"",
		"This command reads YAML command DNA under .zqk/cli/specs (default) and generates",
		"corresponding Go builder files following the versioned builder pattern.",
		"",
		"Do not point --specs-dir at process CAS trees — command DNA lives only under",
		".zqk/cli/specs (paths_config command_specs). --include-process-command-specs is",
		"retained as an escape hatch for any leftover CSPEC-* instances elsewhere.",
		"",
		"The generated builders use the CommandBuilder/CRUDCommandBuilder pattern and",
		"can be used to create cobra.Command instances programmatically.",
	).
		AddExample("Generate builders from CLI DNA (default)", "%s system generate-command-builders --overwrite").
		AddExample("Generate builders from an explicit DNA directory", "%s system generate-command-builders --specs-dir .zqk/cli/specs --overwrite").
		AddExample("Generate builders to a specific output directory", "%s system generate-command-builders --output-dir pkg/cli").
		AddExample("Include leftover process CAS command_spec instances (escape hatch)", "%s system generate-command-builders --include-process-command-specs --overwrite").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-command-builders",
	}
	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(systemProfileHuman)
		if ctx != nil && ctx.Profile != emptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		projectRoot := cli.ResolveProjectRoot(".")
		if specsDir == emptyValue {
			specsDir = filepath.Join(projectRoot, defaultCLISpecsDirRel)
		}
		if outputDir == "" {
			outputDir = filepath.Join(projectRoot, "pkg/cli")
		}

		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		var yamlFiles []string
		err := filepath.Walk(specsDir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if strings.Contains(path, "/scheduler/") || strings.Contains(path, "\\scheduler\\") {
				return nil
			}
			if appledouble.SkipPathInTreeWalk(path) {
				return nil
			}
			if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
				yamlFiles = append(yamlFiles, path)
			}
			return nil
		})
		if err != nil {
			return errfmt.Newf("failed to walk specs directory").Wrap(err)
		}

		logging.Fluent(logger).Info("Starting command builder generation").
			String("specs_dir", specsDir).
			Bool("include_process_command_specs", includeProcessCommandSpecs).
			Log()
		generated := 0
		skipped := 0
		errors := 0

		for _, yamlPath := range yamlFiles {
			relPath, err := filepath.Rel(specsDir, yamlPath)
			if err != nil {
				logging.Fluent(logger).Error(fmt.Sprintf("Failed to get relative path for %s", yamlPath), err).Log()
				errors++
				continue
			}

			data, err := fileutil.ReadFile(yamlPath)
			if err != nil {
				logging.Fluent(logger).Error(fmt.Sprintf("Failed to read %s", yamlPath), err).Log()
				errors++
				continue
			}

			var tempSpec map[string]any
			if err := yaml.Unmarshal(data, &tempSpec); err != nil {
				logging.Fluent(logger).Error(fmt.Sprintf("Failed to parse YAML %s", yamlPath), err).Log()
				errors++
				continue
			}

			if clipkg.IsProcessCommandSpecCAS(tempSpec) && !includeProcessCommandSpecs {
				logging.Fluent(logger).Info(fmt.Sprintf("Skipping process CAS command_spec %s (pass --include-process-command-specs to generate)", relPath)).Log()
				skipped++
				continue
			}

			baseName := clipkg.ResolveCommandBuilderName(tempSpec, yamlPath)
			if baseName == "" || clipkg.IsNumericCASCommandStem(baseName) {
				logging.Fluent(logger).Error(fmt.Sprintf("Refusing numeric/empty builder name for %s", relPath), errfmt.Errorf("unusable command builder stem")).Log()
				errors++
				continue
			}

			outputFile := filepath.Join(outputDir, "bldr_cli_cmd_v1", fmt.Sprintf("%s_command_builder.go", baseName))

			if !overwrite {
				if _, err := fileutil.Stat(outputFile); err == nil {
					logging.Fluent(logger).Debug(fmt.Sprintf("Skipping %s (builder already exists, use --overwrite to replace)", relPath)).Log()
					skipped++
					continue
				}
			}

			if err := clipkg.GenerateCommandBuilderFromYAML(yamlPath, outputDir); err != nil {
				logging.Fluent(logger).Error(fmt.Sprintf("Failed to generate command builder for %s", relPath), err).Log()
				errors++
				continue
			}

			logging.Fluent(logger).Debug(fmt.Sprintf("Generated command builder for %s -> %s", relPath, baseName)).Log()
			generated++
		}

		logging.Fluent(logger).Info(fmt.Sprintf("Summary: Generated %d, Skipped %d, Errors %d", generated, skipped, errors)).Log()

		if errors > 0 {
			return errfmt.Errorf("generation completed with %d errors", errors)
		}

		return nil
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&specsDir, "specs-dir", "", "Directory containing YAML command DNA (default: <project>/.zqk/cli/specs)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for generated builder files (default: <project>/pkg/cli)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing command builder files")
	cmd.Flags().BoolVar(&includeProcessCommandSpecs, "include-process-command-specs", false, "Also generate from process CAS command_spec instances (CSPEC-*); off by default")

	cli.AddCommonFlags(cmd)
	return cmd
}
