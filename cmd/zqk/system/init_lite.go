package system

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/search"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// liteComplianceHooksJSON builds a lightweight PreToolUse hook payload that routes
// run_command invocations through `agent hook pre-command` for zgrep compliance telemetry.
func liteComplianceHooksJSON() []byte {
	exe := brand.ExecutableName()
	return []byte(fmt.Sprintf(`{
  "zgrep-compliance-enforcer": {
    "PreToolUse": [
      {
        "matcher": "run_command",
        "hooks": [
          {
            "type": "command",
            "command": "for p in ./bin/%s ../bin/%s; do if [ -x \"$p\" ]; then exec \"$p\" agent hook pre-command --vendor antigravity; fi; done; exec %s agent hook pre-command --vendor antigravity",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
`, exe, exe, exe))
}

func installLiteComplianceHooks(projectRoot string, force bool) (written, skipped []string, err error) {
	targets := []string{filepath.Join(".agents", "hooks.json")}
	if info, statErr := fileutil.Stat(filepath.Join(projectRoot, ".agent")); statErr == nil && info.IsDir() {
		targets = append(targets, filepath.Join(".agent", "hooks.json"))
	}
	payload := liteComplianceHooksJSON()
	for _, rel := range targets {
		full := filepath.Join(projectRoot, rel)
		if fileutil.Exists(full) && !force {
			skipped = append(skipped, rel)
			continue
		}
		if mkErr := fileutil.MkdirAll(filepath.Dir(full), paths.DirPerm755); mkErr != nil {
			return written, skipped, fmt.Errorf("failed creating hook directory for %s: %w", rel, mkErr)
		}
		if wrErr := fileutil.WriteFile(full, payload, paths.FilePerm644); wrErr != nil {
			return written, skipped, fmt.Errorf("failed writing hook file %s: %w", rel, wrErr)
		}
		written = append(written, rel)
	}
	return written, skipped, nil
}

// runLiteInit executes the zero-bloat Progressive Disclosure onboarding path ("Land with zgrep, Expand to Kernel").
// It creates minimal .zqk/{cache,logs,state} directories, generates multi-host IDE rules, and builds the
// persistent trigram index without seeding the full .zqk/process CAS ontology.
func runLiteInit(cmd *cobra.Command, projectRoot string, force, withMCP, withHooks bool) error {
	if projectRoot == "" {
		cwd, err := fileutil.Getwd()
		if err != nil {
			return fmt.Errorf("failed resolving working directory: %w", err)
		}
		projectRoot = cwd
	}
	if err := paths.ValidateProjectRootNesting(projectRoot); err != nil {
		return fmt.Errorf("cannot initialize lite workspace: %w", err)
	}

	zqkDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	liteDirs := []string{
		filepath.Join(zqkDir, paths.CacheDir),
		filepath.Join(zqkDir, paths.LogsDir),
		filepath.Join(zqkDir, paths.StateDir),
	}
	if err := createDirectoryWithSubdirs(zqkDir, liteDirs); err != nil {
		return fmt.Errorf("failed creating lite workspace directories: %w", err)
	}

	res, err := bldr_cli_cmd_v1.GenerateRules(projectRoot, bldr_cli_cmd_v1.RulesGenerateOptions{
		Hosts: []string{"all"},
		Force: force,
	})
	if err != nil {
		return fmt.Errorf("failed generating multi-host IDE rules: %w", err)
	}

	engine := search.NewEngine(projectRoot)
	if idxErr := engine.BuildTrigramIndex(search.SearchOptions{UseIndex: true}); idxErr != nil {
		return fmt.Errorf("failed building initial trigram index: %w", idxErr)
	}

	mcpWired := false
	if withMCP {
		logger := logging.GetLoggerFromProfile(systemProfileHuman)
		if err := mcp.AutoInstall(projectRoot, logger); err != nil {
			return fmt.Errorf("failed wiring multi-host MCP configuration: %w", err)
		}
		mcpWired = true
	}

	var hooksWritten, hooksSkipped []string
	if withHooks {
		var hookErr error
		hooksWritten, hooksSkipped, hookErr = installLiteComplianceHooks(projectRoot, force)
		if hookErr != nil {
			return fmt.Errorf("failed installing zgrep compliance hooks: %w", hookErr)
		}
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "✓ Initialized ZQK Lite (zgrep-first) workspace in %s\n", projectRoot)
	fmt.Fprintf(out, "  • Zero-bloat footprint: created %s/{cache,logs,state} (no .zqk/process CAS objects seeded)\n", paths.ProjectDataDir)
	fmt.Fprintf(out, "  • Trigram search index warmed and cached for sub-15ms queries\n")
	if mcpWired {
		fmt.Fprintf(out, "  • Multi-host MCP server configuration wired (restart IDE(s) to activate)\n")
	}
	for _, h := range hooksWritten {
		fmt.Fprintf(out, "  • Installed PreToolUse zgrep compliance hook: %s\n", h)
	}
	for _, h := range hooksSkipped {
		fmt.Fprintf(out, "  • Kept existing hook file: %s (use --force to overwrite)\n", h)
	}
	for _, file := range res.Written {
		fmt.Fprintf(out, "  • Wrote IDE rule: %s\n", file)
	}
	for _, file := range res.Skipped {
		fmt.Fprintf(out, "  • Kept existing IDE rule: %s (use --force to overwrite)\n", file)
	}
	fmt.Fprintf(out, "\nNext Steps (60-Second Quickstart):\n")
	fmt.Fprintf(out, "  1. Structural polyglot search:  %s\n", paths.CLIUsage("grep --ast --kind struct"))
	fmt.Fprintf(out, "  2. Token-budgeted agent JSON:   %s\n", paths.CLIUsage("grep 'error' --max-tokens 2000 -f json"))
	fmt.Fprintf(out, "  3. Pilot ROI compliance report: %s\n", paths.CLIUsage("grep --roi"))
	fmt.Fprintf(out, "  4. Expand to full Knowledge Kernel anytime: %s\n", paths.CLIUsage("init --merge"))
	return nil
}
