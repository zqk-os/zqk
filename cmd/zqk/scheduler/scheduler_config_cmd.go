package scheduler

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

const schedulerProfileHuman = "human"

// NewConfigCmd creates the scheduler config command (get/set .zqk/scheduler/config.yaml)
// Command structure and flags from .zqk/cli/specs/scheduler/config_command.yaml (generate-command-builders)
func NewConfigCmd() *cobra.Command {
	configCmd := bldr_cli_cmd_v1.NewSchedulerConfigCommandBuilder()
	configCmd.RunE = runConfig
	return configCmd
}

func runConfig(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != emptyValue {
			projectRoot = ctx.ProjectRoot
		}
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root is required (run from project directory or set ZQK_PROJECT_ROOT)")
	}

	config, err := loadSchedulerConfig(projectRoot)
	if err != nil {
		return errfmt.Newf("failed to load scheduler config").Wrap(err)
	}

	jobsPausedSet, _ := cmd.Flags().GetBool("jobs-paused")
	noJobsPausedSet, _ := cmd.Flags().GetBool("no-jobs-paused")
	if jobsPausedSet && noJobsPausedSet {
		return errfmt.Errorf("cannot set both --jobs-paused and --no-jobs-paused")
	}

	profile := schedulerProfileHuman
	if ctx := cli.GetContext(cmd); ctx != nil {
		profile = ctx.Profile
	}
	logger := logging.GetLoggerFromProfile(profile)

	if jobsPausedSet {
		config.JobsPaused = true
		if err := saveSchedulerConfig(projectRoot, config); err != nil {
			return errfmt.Newf("failed to save scheduler config").Wrap(err)
		}
		_ = schedulerpkg.WriteReloadConfigRequest(projectRoot) // notify daemon so it picks up within seconds
		schedulerpkg.SLog(logger).Info("Set jobs_paused=true; running daemon will pick up within a few seconds (no restart required)").
			String("config_path", filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerConfigFile)).
			Log()
	} else if noJobsPausedSet {
		config.JobsPaused = false
		if err := saveSchedulerConfig(projectRoot, config); err != nil {
			return errfmt.Newf("failed to save scheduler config").Wrap(err)
		}
		_ = schedulerpkg.WriteReloadConfigRequest(projectRoot) // notify daemon so it reschedules within seconds
		schedulerpkg.SLog(logger).Info("Set jobs_paused=false; running daemon will reschedule timer/immediate jobs within a few seconds (no restart required)").
			String("config_path", filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerConfigFile)).
			Log()
	}

	// Output current config (structured for scripts; respects --format)
	out := map[string]any{
		objects.FieldKeyEnabled: config.Enabled,
		"project_type":          config.ProjectType,
		"jobs_paused":           config.JobsPaused,
	}
	return cli.FormatOutput(cmd, out)
}
