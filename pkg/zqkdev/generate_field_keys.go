package zqkdev

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// NewGenerateFieldKeysCmd generates pkg/objects/field_keys.go from the union of all field
// names in object_specs (ResolvedFields per kind), merged with supplemental keys in
// pkg/objects/field_keys_generate.go.
func NewGenerateFieldKeysCmd() *cobra.Command {
	var (
		specsDir string
		output   string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate field_keys.go from object specs",
		"Regenerate pkg/objects/field_keys.go with a union of all YAML field names across kinds.",
		"",
		"Uses the same spec loading path as generate-spec-index (LoadSpecWithInheritance).",
		"After changing object_specs, run this command then commit the generated file.",
		"The pre-commit script scripts/check-field-key-literals.sh reads keys from field_keys.go.",
	).
		AddExample("Regenerate with defaults", "%s system generate-field-keys").
		AddExample("Custom specs dir", "%s system generate-field-keys --specs-dir "+paths.ProcessInternalObjectSpecsDir).
		AddExample("Custom output path", "%s system generate-field-keys --output pkg/objects/field_keys.go").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-field-keys",
	}

	cli.RequireSession(cmd, false)
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(SystemProfileHuman)
		if ctx != nil && ctx.Profile != EmptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		if specsDir == EmptyValue {
			pathsConfig := validation.GetGlobalPathsConfig()
			specsDir = pathsConfig.FindPath("object_specs")
			if specsDir == EmptyValue {
				return errfmt.Errorf("could not resolve object_specs directory via paths config")
			}
		}

		projectRoot := ""
		if ctx != nil && ctx.ProjectRoot != EmptyValue {
			projectRoot = ctx.ProjectRoot
		}
		if projectRoot == EmptyValue {
			if wd, err := fileutil.Getwd(); err == nil {
				projectRoot = wd
			}
		}

		if output == EmptyValue {
			if projectRoot != EmptyValue {
				output = filepath.Join(projectRoot, "pkg", "objects", "field_keys.go")
			} else {
				abs, err := filepath.Abs(filepath.Join("pkg", "objects", "field_keys.go"))
				if err != nil {
					return errfmt.Newf("resolve output path").Wrap(err)
				}
				output = abs
			}
		} else if !filepath.IsAbs(output) {
			base := projectRoot
			if base == EmptyValue {
				var err error
				base, err = fileutil.Getwd()
				if err != nil {
					return errfmt.Newf("getwd").Wrap(err)
				}
			}
			output = filepath.Join(base, output)
		}

		logging.Fluent(logger).Info("Generating field keys").
			String("specs_dir", specsDir).
			String("output", output).
			Log()

		src, err := objects.GenerateFieldKeysGoSource(specsDir)
		if err != nil {
			return errfmt.Newf("generate field_keys.go").Wrap(err)
		}

		if err := fileutil.WriteFile(output, src, paths.FilePerm644); err != nil { //nolint:gosec // Generated source; 0600 matches other codegen outputs
			return errfmt.Errorf("write %s: %w", output, err)
		}

		names, err := objects.BuildUnionFieldKeyNames(specsDir)
		if err != nil {
			return errfmt.Newf("count keys").Wrap(err)
		}

		logging.Fluent(logger).Info("field_keys.go written").
			String("output", output).
			Int("key_count", len(names)).
			Log()

		return nil
	})

	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().StringVar(&specsDir, "specs-dir", "", "Directory containing YAML spec files (default: object_specs from paths config)")
	cmd.Flags().StringVar(&output, "output", "", "Output path (default: <project>/pkg/objects/field_keys.go)")
	cli.AddCommonFlags(cmd)
	return cmd
}
