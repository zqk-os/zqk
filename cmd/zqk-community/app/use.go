// Package app: use command — set persistent project root for this workspace.
// Ensures scope is explicit and prevents data/process collision across nested roots.
// See docs/architecture/PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md.
// Command structure from spec: .zqk/cli/specs/project/use_command.yaml (builder: bldr_cli_cmd_v1.NewProjectUseCommandBuilder).

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	clicontext "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	schedulerpkg "github.com/lanceman/zqk/pkg/scheduler"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// NewUseCmd returns the use command (set persistent project root).
// Built from spec builder for uniformity and codegen; RunE is wired here.
func NewUseCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewProjectUseCommandBuilder()
	cmd.Args = cobra.ArbitraryArgs
	cmd.Long = "Sets the project root for this workspace. Path must be under the workspace (repo) and contain .zqk. Stored in .zqk/current_root. Stops the previous root's scheduler and starts the scheduler for the new root. Use --no-swap to run in a nested context without changing the persisted root or touching the scheduler.\n\nTo use the directory you're in without ambiguity, run 'zqk use $(pwd)' — the shell expands the path before zqk runs. If you pass '.' the path is your current working directory (CWD), so run from the directory you want (e.g. 'cd /path/to/repo && zqk use .'). With nested roots (e.g. test-scenarios/... with their own .zqk), running 'zqk use .' from a nested dir will set that nested dir as the project root until you run 'zqk use $(pwd)' or 'zqk use .' again from the intended root."
	cmd.RunE = runUse
	// --no-swap is added by the generated builder from the command spec (use_command.yaml flags)
	return cmd
}

func runUse(cmd *cobra.Command, args []string) error {
	noSwap, _ := cmd.Flags().GetBool("no-swap")

	// Parse args: path [--] command...
	// "zqk use path --no-swap -- go test ./..." => pathArg=path, commandArgs=["go","test","./..."]
	var pathArg string
	var commandArgs []string
	for i, a := range args {
		if a == "--" {
			if i == 0 {
				return errfmt.Errorf("path is required before --")
			}
			pathArg = args[0]
			commandArgs = args[i+1:]
			break
		}
	}
	if pathArg == EmptyValue {
		if len(args) > 0 {
			pathArg = args[0]
			if noSwap && len(args) > 1 {
				commandArgs = args[1:]
			}
		} else {
			pathArg = "."
		}
	}

	if !noSwap && len(args) > 1 {
		return errfmt.Errorf("use without --no-swap accepts at most one path; use 'zqk use <path> --no-swap -- <command>' to run a command in a nested root")
	}

	// POL-CODE-007: use logger from context for user-facing messages
	profile := profileHuman
	if c := cli.GetContext(cmd); c != nil {
		profile = c.Profile
	}
	logger := logging.GetLoggerFromProfile(profile)

	workspaceRoot := clicontext.FindWorkspaceRoot(".")
	if workspaceRoot == EmptyValue {
		return errfmt.Errorf("not inside a workspace (no .zqk found); run from the repo or a directory under it")
	}

	absPath, err := filepath.Abs(pathArg)
	if err != nil {
		return errfmt.Newf("resolve path").Wrap(err)
	}

	// Interpret pathArg as either a settings file or a directory containing zqk-settings.yaml.
	settingsPath := absPath
	info, err := fileutil.Stat(settingsPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			// If user passed a directory that does not exist, or a non-existent file path, fail with clear message.
			return errfmt.Errorf("brand settings file not found at %s (expected %s or a directory containing it)", settingsPath, paths.BrandSettingsFilename)
		}
		return errfmt.Newf("stat path").Wrap(err)
	}
	if info.IsDir() {
		settingsPath = filepath.Join(settingsPath, paths.BrandSettingsFilename)
		if _, err := fileutil.Stat(settingsPath); err != nil {
			if fileutil.IsNotExist(err) {
				return errfmt.Errorf("brand settings file not found at %s", settingsPath)
			}
			return errfmt.Newf("stat settings file").Wrap(err)
		}
	}

	// Load settings and derive project root from settings.Paths.ProjectRoot or the file directory.
	_, projectRoot, err := clicontext.LoadBrandSettingsFromFile(settingsPath)
	if err != nil {
		return errfmt.Newf("brand settings invalid").Wrap(err)
	}

	if !paths.UnderProjectRoot(workspaceRoot, projectRoot) {
		return errfmt.Errorf("derived project root must be under the workspace (no escape): %s", projectRoot)
	}

	if noSwap {
		// Do not persist root; do not stop/start scheduler.
		if len(commandArgs) == 0 {
			logging.Fluent(logger).Info("Settings valid (no-swap); persisted root and scheduler unchanged.").
				String("settings_path", settingsPath).
				ProjectRoot(projectRoot).
				Log()
			logging.Fluent(logger).Info("To run a command in this root without swapping: zqk use " + pathArg + " --no-swap -- <command>").Log()
			return nil
		}
		// Run command with ZQK_TEST_ROOT so the child uses this root and does not touch current_root.
		env := os.Environ()
		env = setOrReplaceEnv(env, zqkenv.TestRoot().Name(), projectRoot)
		//nolint:gosec // G204: command/args are from user's explicit "zqk use <path> --no-swap -- <cmd> [args]"; user controls their own process
		c := execwrap.Command(commandArgs[0], commandArgs[1:]...)
		c.Env = env
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		zqkenv.WireExecForIsolatedProject(c, workspaceRoot)
		if err := c.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			return err
		}
		return nil
	}

	// Swap: persist current_root and align scheduler using the project root derived from settings.
	logging.Fluent(logger).Info("Setting project root from settings").
		String("settings_path", settingsPath).
		ProjectRoot(projectRoot).
		Log()
	previousRoot := clicontext.ReadPersistedCurrentRoot(workspaceRoot)
	if previousRoot != EmptyValue && previousRoot != projectRoot {
		if running, _, _ := schedulerpkg.IsSchedulerRunning(previousRoot); running {
			_ = schedulerpkg.StopSchedulerByPID(previousRoot)
			logging.Fluent(logger).Info("Stopped scheduler for previous project root").Log()
		}
	}

	if err := clicontext.WritePersistedCurrentRoot(workspaceRoot, projectRoot); err != nil {
		return err
	}
	clicontext.NotifyCurrentRootSet(workspaceRoot, projectRoot, previousRoot)

	// Refresh path alias cache for the new root so path resolution works without pre-warm.
	storagepkg.BuildPathAliasCacheForProject(projectRoot)

	logging.Fluent(logger).Info("Project root set").
		ProjectRoot(projectRoot).
		Log()
	return nil
}

// setOrReplaceEnv returns a copy of env with key=value set (replacing any existing key).
func setOrReplaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	out = append(out, prefix+value)
	return out
}
