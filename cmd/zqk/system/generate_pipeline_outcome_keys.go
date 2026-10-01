package system

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewGeneratePipelineOutcomeKeysCmd wires command builder from spec:
// .zqk/cli/specs/system/generate_pipeline_outcome_keys_command.yaml
func NewGeneratePipelineOutcomeKeysCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "generate-pipeline-outcome-keys"}
	cli.RequireSession(cmd, false)
	cli.BindAsyncProgress(cmd, runGeneratePipelineOutcomeKeys)
	return cmd
}

func runGeneratePipelineOutcomeKeys(cmd *cobra.Command, _ []string) error {
	ctx := cli.GetContext(cmd)
	logger := logging.GetLoggerFromProfile(systemProfileHuman)
	if ctx != nil && ctx.Profile != emptyValue {
		logger = logging.GetLoggerFromProfile(ctx.Profile)
	}

	projectRoot := ""
	if ctx != nil && ctx.ProjectRoot != emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == emptyValue {
		if wd, err := fileutil.Getwd(); err == nil {
			projectRoot = wd
		}
	}

	registryPath, _ := cmd.Flags().GetString("registry")
	output, _ := cmd.Flags().GetString("output")

	if registryPath == emptyValue {
		if projectRoot != emptyValue {
			registryPath = filepath.Join(projectRoot, paths.ProcessInternalPipelineOutcomeKeysFile)
		} else {
			abs, err := filepath.Abs(paths.ProcessInternalPipelineOutcomeKeysFile)
			if err != nil {
				return errfmt.Newf("resolve registry path").Wrap(err)
			}
			registryPath = abs
		}
	} else if !filepath.IsAbs(registryPath) {
		base := projectRoot
		if base == emptyValue {
			var err error
			base, err = fileutil.Getwd()
			if err != nil {
				return errfmt.Newf("getwd").Wrap(err)
			}
		}
		registryPath = filepath.Join(base, registryPath)
	}

	if output == emptyValue {
		if projectRoot != emptyValue {
			output = filepath.Join(projectRoot, "pkg", "pipeline", "outcome_keys.go")
		} else {
			abs, err := filepath.Abs(filepath.Join("pkg", "pipeline", "outcome_keys.go"))
			if err != nil {
				return errfmt.Newf("resolve output path").Wrap(err)
			}
			output = abs
		}
	} else if !filepath.IsAbs(output) {
		base := projectRoot
		if base == emptyValue {
			var err error
			base, err = fileutil.Getwd()
			if err != nil {
				return errfmt.Newf("getwd").Wrap(err)
			}
		}
		output = filepath.Join(base, output)
	}

	logging.Fluent(logger).Info("Generating pipeline outcome keys").
		String("registry", registryPath).
		String("output", output).
		Log()

	names, err := pipeline.LoadOutcomeKeyNamesFromYAML(registryPath)
	if err != nil {
		return errfmt.Newf("load outcome key registry").Wrap(err)
	}

	relReg := paths.ProcessInternalPipelineOutcomeKeysFile
	src, err := pipeline.GenerateOutcomeKeysGoSource(relReg, names)
	if err != nil {
		return errfmt.Newf("generate outcome_keys.go").Wrap(err)
	}

	if err := fileutil.WriteFile(output, src, paths.FilePerm644); err != nil { //nolint:gosec // Generated source; 0600 matches other codegen
		return errfmt.Errorf("write %s: %w", output, err)
	}

	logging.Fluent(logger).Info("outcome_keys.go written").
		String("output", output).
		Int("key_count", len(names)).
		Log()

	return nil
}
