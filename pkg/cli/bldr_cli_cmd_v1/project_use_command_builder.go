package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewProjectUseCommandBuilder creates a new project_use command
func NewProjectUseCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("use")
	builder.WithShort("Set persistent project root for this workspace")
	help := clipkg.DynamicHelpBuilder("Set persistent project root for this workspace")
	help.WithDescriptionLines("Orients the CLI using a brand settings file (zqk-settings.yaml) and sets the persistent project root for this workspace. The path argument is interpreted as either a settings file path or a directory containing zqk-settings.yaml. The file is loaded and validated against the brand settings schema, and the project root is derived from paths.project_root (or the file’s directory).")
	help.WithDescriptionLines("The derived project root is stored in the workspace's .zqk/current_root so the same binary can switch between core repo and nested roots (e.g. test-scenarios) without env vars. Without a valid settings file, the scheduler and other services will not run and will point back to 'zqk use <settings-path>'.")
	help.WithDescriptionLines("When you switch to a different root, the scheduler for the previous root is stopped and the scheduler for the new root is started automatically.")
	help.WithDescriptionLines("Use --no-swap when you want to run in a nested context without changing the persisted root or touching the scheduler (e.g. tests or one-off commands). With --no-swap, optional trailing arguments form the command to run (use -- to separate path from command: zqk use <path> --no-swap -- <command...>). The child runs with ZQK_TEST_ROOT set to the derived project root.")
	help.AddExample("Use current directory explicitly (recommended; shell expands path, no CWD ambiguity)", "%s use $(pwd)")
	help.AddExample("Use current directory ('.' is CWD-relative; run from the dir you want)", "%s use .")
	help.AddExample("Use a scenario under the repo", "%s use test-scenarios/onboarding-evaluation")
	help.AddExample("Run tests in a scenario root without swapping (persisted root and scheduler unchanged)", "%s use test-scenarios/foo --no-swap -- go test ./...")
	help.AddExample("Validate a path as project root without persisting", "%s use test-scenarios/foo --no-swap")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("no-swap", "", false, "Do not persist root or touch scheduler. Validate path only, or run a command with this root (e.g. zqk use test-scenarios/foo --no-swap -- go test ./...).")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	cli.RequireStorage(cmd, false)
	cli.RequireSession(cmd, false)
	cli.RequireSchedulerCheck(cmd, false)
	return cmd
}
