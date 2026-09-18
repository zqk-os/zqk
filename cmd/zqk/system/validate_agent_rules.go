package system

// Command spec: .zqk/cli/specs/system/validate_agent_rules_command.yaml (generate-command-builders).

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentrules"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/spf13/cobra"
)

// NewValidateAgentRulesCmd validates that .zqk/specs/configs/agent_rules_manifest.yaml
// matches *.mdc files under the configured agent rules directory (see agentrules.DefaultAgentRulesRelativePath).
// Typical use: run from the repo root with no flags, or --write-manifest alone to refresh the registry.
// Alias validate-ide-rules is legacy compatibility only.
func NewValidateAgentRulesCmd() *cobra.Command {
	var (
		writeManifest bool
		projectRoot   string
		rulesDir      string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Validate IDE agent rules manifest vs rules directory",
		"Ensures the repo registry YAML matches *.mdc files on disk.",
		"",
		"Happy path (no flags): run from the project root — validate matches the default rules directory,",
		"or pass --write-manifest alone to regenerate the manifest after adding/renaming rule files.",
		"Optional --rules-dir / AGENT_RULES_DIR only when rules are not under "+agentrules.DefaultAgentRulesRelativePath+".",
		"If a workflow ever needs many steps or flags, prefer adding a thin veneer command rather than typing them every time.",
		"Canonical manifest: "+paths.ProcessInternalConfigsDir+"/"+agentrules.ManifestFileName+".",
	).
		AddExample("Validate (no flags)", "%s system validate-agent-rules").
		AddExample("Refresh manifest after editing rules (one flag)", "%s system validate-agent-rules --write-manifest").
		AddExample("Alternate rules directory", "%s system validate-agent-rules --rules-dir .agent/rules").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemValidateAgentRulesCommandBuilder(), &cobra.Command{
		Use:     "validate-agent-rules",
		Aliases: []string{"validate-ide-rules"},
	})

	cli.RequireSession(cmd, false)
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(systemProfileHuman)
		if ctx != nil && ctx.Profile != emptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		root := projectRoot
		if root == emptyValue && ctx != nil && ctx.ProjectRoot != emptyValue {
			root = ctx.ProjectRoot
		}
		root = ProjectRootOrResolveDot(root)

		if writeManifest {
			if err := agentrules.WriteManifestFromDisk(root, rulesDir); err != nil {
				return err
			}
			logging.Fluent(logger).Info("Wrote agent rules manifest").
				String("path", agentrules.ManifestPath(root)).
				String("rules_dir", agentrules.ResolveRulesDir(root, rulesDir)).
				Log()
			return nil
		}
		if err := agentrules.Validate(root, rulesDir); err != nil {
			return errfmt.Newf("agent rules validation failed").Wrap(err)
		}
		logging.Fluent(logger).Info("Agent rules manifest matches rules directory").
			String("rules_dir", agentrules.ResolveRulesDir(root, rulesDir)).
			Log()
		return nil
	})

	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().BoolVar(&writeManifest, "write-manifest", false, "Rewrite manifest from current *.mdc files (sorted)")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root (default: resolve from cwd)")
	cmd.Flags().StringVar(&rulesDir, "rules-dir", "", "Optional rules directory relative to project root (default "+agentrules.DefaultAgentRulesRelativePath+"; overrides AGENT_RULES_DIR)")
	cli.AddCommonFlags(cmd)

	return cmd
}
