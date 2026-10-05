package scheduler

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// NewSkipWindowCmd manages .zqk/scheduler/skip_window.yaml for bulk policy skips during instability.
// Command structure from .zqk/cli/specs/scheduler/skip_window_command.yaml and scheduler/skip_window/*.yaml (generate-command-builders).
func NewSkipWindowCmd() *cobra.Command {
	parent := bldr_cli_cmd_v1.NewSchedulerSkipWindowCommandBuilder()
	setCmd := bldr_cli_cmd_v1.NewSchedulerSkipWindowSetCommandBuilder()
	setCmd.RunE = runSkipWindowSet
	clearCmd := bldr_cli_cmd_v1.NewSchedulerSkipWindowClearCommandBuilder()
	clearCmd.RunE = runSkipWindowClear
	statusCmd := bldr_cli_cmd_v1.NewSchedulerSkipWindowStatusCommandBuilder()
	statusCmd.RunE = runSkipWindowStatus
	parent.AddCommand(setCmd, clearCmd, statusCmd)
	return parent
}

func resolveSchedulerSkipWindowDir(cmd *cobra.Command) (string, error) {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != emptyValue {
			projectRoot = ctx.ProjectRoot
		}
	}
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root is required")
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir), nil
}

func runSkipWindowSet(cmd *cobra.Command, _ []string) error {
	schedDir, err := resolveSchedulerSkipWindowDir(cmd)
	if err != nil {
		return err
	}
	forStr, _ := cmd.Flags().GetString("for")
	forStr = strings.TrimSpace(forStr)
	untilStr, _ := cmd.Flags().GetString("until")
	untilStr = strings.TrimSpace(untilStr)
	if forStr != emptyValue && untilStr != emptyValue {
		return errfmt.Errorf("use either --for or --until, not both")
	}
	var until time.Time
	switch {
	case forStr != emptyValue:
		d, err := time.ParseDuration(forStr)
		if err != nil {
			return errfmt.Errorf("parse --for duration: %w", err)
		}
		until = time.Now().UTC().Add(d)
	case untilStr != emptyValue:
		var err error
		until, err = time.Parse(time.RFC3339, untilStr)
		if err != nil {
			return errfmt.Errorf("parse --until as RFC3339: %w", err)
		}
	default:
		return errfmt.Errorf("set --for (duration) or --until (RFC3339)")
	}
	if !time.Now().UTC().Before(until) {
		return errfmt.Errorf("--until/--for must end in the future")
	}

	matchAll, _ := cmd.Flags().GetBool("match-all")
	jobTypes, _ := cmd.Flags().GetStringSlice("job-type")
	triggerTypes, _ := cmd.Flags().GetStringSlice("trigger-type")
	titleContains, _ := cmd.Flags().GetString("title-contains")
	titleContains = strings.TrimSpace(titleContains)
	if !matchAll && len(jobTypes) == 0 && len(triggerTypes) == 0 && titleContains == emptyValue {
		return errfmt.Errorf("specify --match-all and/or --job-type / --trigger-type / --title-contains filters")
	}

	sw := &schedulerpkg.SchedulerSkipWindow{
		Until:         until,
		MatchAll:      matchAll,
		JobTypes:      jobTypes,
		TriggerTypes:  triggerTypes,
		TitleContains: titleContains,
	}
	if err := schedulerpkg.SaveSchedulerSkipWindow(schedDir, sw); err != nil {
		return errfmt.Newf("save skip_window").Wrap(err)
	}
	profile := string(pkgctx.ProfileSystem)
	if ctx := cli.GetContext(cmd); ctx != nil && ctx.Profile != emptyValue {
		profile = ctx.Profile
	}
	logger := logging.GetLoggerFromProfile(profile)
	schedulerpkg.SLog(logger).Info("Wrote scheduler skip window").
		Path(filepath.Join(schedDir, "skip_window.yaml")).
		String("until", until.UTC().Format(time.RFC3339)).
		Log()
	msg := fmt.Sprintf("skip_window active until %s (%s)\n", until.UTC().Format(time.RFC3339), schedDir)
	if err := cli.WriteOutput(cmd, []byte(msg)); err != nil {
		return err
	}
	return nil
}

func runSkipWindowClear(cmd *cobra.Command, _ []string) error {
	schedDir, err := resolveSchedulerSkipWindowDir(cmd)
	if err != nil {
		return err
	}
	if err := schedulerpkg.ClearSchedulerSkipWindow(schedDir); err != nil {
		return err
	}
	if err := cli.WriteOutput(cmd, []byte("skip_window cleared\n")); err != nil {
		return err
	}
	return nil
}

func runSkipWindowStatus(cmd *cobra.Command, _ []string) error {
	schedDir, err := resolveSchedulerSkipWindowDir(cmd)
	if err != nil {
		return err
	}
	sw, _ := schedulerpkg.LoadSchedulerSkipWindow(schedDir, time.Now().UTC())
	if sw == nil {
		if err := cli.WriteOutput(cmd, []byte("no active skip_window (missing, expired, or no filters)\n")); err != nil {
			return err
		}
		return nil
	}
	statusMsg := fmt.Sprintf("until: %s\nmatch_all: %v\njob_types: %v\ntrigger_types: %v\ntitle_contains: %q\n",
		sw.Until.UTC().Format(time.RFC3339), sw.MatchAll, sw.JobTypes, sw.TriggerTypes, sw.TitleContains)
	if err := cli.WriteOutput(cmd, []byte(statusMsg)); err != nil {
		return err
	}
	return nil
}
