package system

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewInitCmd creates a new init command
func NewInitCmd() *cobra.Command {
	var (
		projectName           string
		template              string
		force                 bool
		snapshotPath          string
		answerFilePath        string
		legacy                bool
		merge                 bool
		wipe                  bool
		discover              bool
		withMaintenanceJobs   bool
		withOnboardingRoadmap bool
		simple                bool
		advanced              bool
		importOntology        string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Initialize a new ZQK project",
		"Initialize a new ZQK project in the current directory.",
		"",
		"Supports three initialization scenarios:",
		"  1. Greenfield: Brand new project (default)",
		"  2. Legacy: Existing project being brought under ZQK management",
		"  3. Snapshot: Initialize from snapshot data (test scenarios)",
		"",
		"This command creates the necessary directory structure and configuration files",
		"for a new ZQK project. It sets up:",
		"  - .zqk/ directory for configuration and state",
		"  - "+paths.ProcessDir+"/ directory structure for object storage",
		"  - Initial configuration files",
	).
		AddExample("Greenfield: Initialize in current directory (Note: 'system init' is an alias)", "%s init").
		AddExample("Declarative Seed: Initialize with answer file", "%s init --answer-file seed.yaml").
		AddExample("Legacy: Initialize existing project", "%s init --legacy").
		AddExample("Snapshot: Initialize from snapshot (test scenario)", zqkenv.TestRoot().Name()+"=test-scenarios/my-scenario %s init --from-snapshot snapshot.csnap --wipe").
		AddExample("Snapshot: Merge with existing data", "%s init --from-snapshot snapshot.csnap --merge").
		AddExample("Legacy + discover: Initialize and report existing objects", "%s init --legacy --discover").
		AddExample("Greenfield with maintenance jobs", "%s init --with-maintenance-jobs").
		AddExample("Greenfield with onboarding curriculum", "%s init --with-onboarding-roadmap").
		ExcludeCommonFlags()

	initCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemInitCommandBuilder(), &cobra.Command{
		Use: "init",
	})
	cli.RequireSession(initCmd, false)
	cli.RequireStorage(initCmd, false)
	cli.RequireSchedulerCheck(initCmd, false)

	cli.BindAsyncProgress(initCmd, func(cmd *cobra.Command, args []string) error {
		projectName, _ = cmd.Flags().GetString("project-name")
		template, _ = cmd.Flags().GetString("template")
		force, _ = cmd.Flags().GetBool("force")
		snapshotPath, _ = cmd.Flags().GetString("from-snapshot")
		answerFilePath, _ = cmd.Flags().GetString("answer-file")
		legacy, _ = cmd.Flags().GetBool("legacy")
		merge, _ = cmd.Flags().GetBool("merge")
		wipe, _ = cmd.Flags().GetBool("wipe")
		discover, _ = cmd.Flags().GetBool("discover")
		withMaintenanceJobs, _ = cmd.Flags().GetBool("with-maintenance-jobs")
		withOnboardingRoadmap, _ = cmd.Flags().GetBool("with-onboarding-roadmap")
		simple, _ = cmd.Flags().GetBool("simple")
		advanced, _ = cmd.Flags().GetBool("advanced")
		importOntology, _ = cmd.Flags().GetString("import-ontology")
		return runInit(cmd, projectName, template, force, snapshotPath, answerFilePath, legacy, merge, wipe, discover, withMaintenanceJobs, withOnboardingRoadmap, simple, advanced, importOntology)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(initCmd)

	return initCmd
}

// Helper functions are defined here and shared with init_impl.go

func createProjectDataDir(projectDataDir string, force bool) error {
	// Create project data directory (typically .zqk)
	if err := fileutil.MkdirAll(projectDataDir, paths.DirPerm755); err != nil {
		return err
	}

	if force {
		// Clean stale runtime socket and PID files on forced init
		runDir := filepath.Join(projectDataDir, paths.RunSubdir)
		_ = fileutil.RemoveAll(runDir)
	}

	// Create subdirectories: canonical .zqk layout (logs, metrics, cache, scheduler, wal) with clear separation.
	subdirs := []string{
		filepath.Join(projectDataDir, paths.CacheDir),
		filepath.Join(projectDataDir, paths.StateDir),
		filepath.Join(projectDataDir, paths.LogsDir),
		filepath.Join(projectDataDir, paths.LogsDir, paths.LogsReportsSubdir),
		filepath.Join(projectDataDir, paths.MetricsDir),
		filepath.Join(projectDataDir, paths.SchedulerDir),
		filepath.Join(projectDataDir, paths.WalDir),
	}

	return createDirectoryWithSubdirs(projectDataDir, subdirs)
}

func createDirectoryWithSubdirs(baseDir string, subdirs []string) error {
	if err := fileutil.MkdirAll(baseDir, paths.DirPerm755); err != nil {
		return err
	}
	for _, dir := range subdirs {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return err
		}
	}
	return nil
}

func createProcessDir(processDir string, _ bool) error {
	// Seed essential directories for baseline workspace initialization
	subdirs := []string{
		filepath.Join(processDir, "backlog_items"),
		filepath.Join(processDir, "policies"),
	}

	return createDirectoryWithSubdirs(processDir, subdirs)
}

// writeProjectConfigFiles writes project YAML to config/zqk.yaml (SSOT).
func writeProjectConfigFiles(projectDataDir, projectName, template string, force bool) error {
	projectRoot := filepath.Dir(projectDataDir)
	canonicalPath := filepath.Join(projectRoot, paths.ConfigDir, paths.ZqkConfigFileName)
	if _, err := fileutil.Stat(canonicalPath); err == nil && !force {
		return errfmt.Errorf("config file already exists: %s (use --force to overwrite)", canonicalPath)
	}
	content := buildProjectConfigContent(projectName, template)
	if err := fileutil.MkdirAll(filepath.Dir(canonicalPath), paths.DirPerm755); err != nil {
		return err
	}
	return fileutil.WriteFile(canonicalPath, []byte(content), paths.FilePerm644) //nolint:gosec // Config - 0600 acceptable
}

// writeMCPConfigFile writes a default MCP configuration to .zqk/mcp/config.yaml
// This enables alias mode by default to prevent MCP clients from choking on 1000+ tools
func writeMCPConfigFile(projectDataDir string, force bool) error {
	mcpDir := filepath.Join(projectDataDir, paths.MCPDir)
	if err := fileutil.MkdirAll(mcpDir, paths.DirPerm755); err != nil {
		return err
	}

	configPath := filepath.Join(mcpDir, paths.MCPConfigFile)
	if _, err := fileutil.Stat(configPath); err == nil && !force {
		return nil // Don't overwrite existing
	}

	// G6a: alias_mode + register_cli_tools=false exposes built-in product tools
	// (object_*, system_*, workflow helpers) without flooding IDE with CLI leaves.
	content := `# Generated by system init
mcp_server:
  # Built-in product tools only (INCLUDE surface). CLI leaf discovery stays off.
  register_cli_tools: false
  tools:
    # Hide full CLI command tree; expose built-in/alias tools (object/system/workflow).
    # Prevents IDEs like IDE from failing due to oversized tool catalogs.
    alias_mode: true
`
	return fileutil.WriteFile(configPath, []byte(content), paths.FilePerm644)
}

func writeSystemAccount(projectRoot string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	accID := pkgctx.SystemAccountID
	content := fmt.Sprintf(`id: %s
kind: account
schema_version: 2.0.0
namespace_id: zqk:kernel
title: System Account
username: system
status: active
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, accID, accID, accID, now, now)
	accountsDir := filepath.Join(projectRoot, paths.ProcessDir, "accounts")
	if err := fileutil.MkdirAll(accountsDir, paths.DirPerm755); err != nil {
		return err
	}
	cas := filecas.NewContentAddressableStorage(accountsDir, objects.KindAccount)

	// Clean up and migrate legacy unindexed files (system.yaml or {id}.yaml) if present.
	legacyNames := []string{"system.yaml", accID + ".yaml"}
	for _, legName := range legacyNames {
		legPath := filepath.Join(accountsDir, legName)
		if data, err := fileutil.ReadFile(legPath); err == nil {
			_ = cas.Create(accID, data)
			_ = fileutil.Remove(legPath)
		}
	}

	if hash, err := cas.GetIndex().GetHash(accID); err == nil && hash != "" {
		credPath := paths.CredentialsPath(projectRoot)
		if !fileutil.Exists(credPath) {
			_ = fileutil.EnsureDir(filepath.Dir(credPath))
			_ = fileutil.WriteSecureFile(credPath, []byte(accID+"\n"))
		}
		return nil
	}
	if err := cas.Create(accID, []byte(content)); err != nil {
		return err
	}
	credPath := paths.CredentialsPath(projectRoot)
	if !fileutil.Exists(credPath) {
		_ = fileutil.EnsureDir(filepath.Dir(credPath))
		_ = fileutil.WriteSecureFile(credPath, []byte(accID+"\n"))
	}
	return nil
}

func buildProjectConfigContent(projectName, template string) string {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	return fmt.Sprintf(`# Generated by %s system init. Committed defaults.
# Override in config/zqk-local.yaml (do not copy from another kernel).
# This file is the project YAML SSOT. Machine lite-files live in .zqk/agent-runtime/.
brand:
  executable_name: %s
  product_name: ZQK
system:
  storage_mode: "file"
  stream_storage_enabled: true
kernel_state:
  project_root: ""

project:
  name: %s
  template: %s

storage:
  backend: file
  data_dir: %s

logging:
  profile: human
  level: info

cache:
  enabled: true
  ttl_seconds: 3600

# Fail-fast validation budgets. Healthy objects finish in ms; multi-second
# kind_overrides hide lock contention. Prefer fixing stalls over raising budgets.
validation:
  per_object_timeout:
    default_seconds: 5
    kind_overrides: {}
  stuck_timeout_seconds: 30
`, cliCmd, cliCmd, projectName, template, paths.ProcessDir)
}

func createConfigFile(configPath, projectName, template string, force bool) error {
	if _, err := fileutil.Stat(configPath); err == nil && !force {
		return errfmt.Errorf("config file already exists: %s (use --force to overwrite)", configPath)
	}
	return fileutil.WriteFile(configPath, []byte(buildProjectConfigContent(projectName, template)), paths.FilePerm644) //nolint:gosec // Config - 0600 acceptable
}

var defaultRequiredGitignorePatterns = []string{
	paths.ProjectDataDir + "/cache/",
	paths.ProjectDataDir + "/state/",
	paths.ProjectDataDir + "/logs/",
	paths.ProjectDataDir + "/scheduler/",
	paths.ProjectDataDir + "/wal/",
	paths.ProjectDataDir + "/metrics/",
	paths.ProjectDataDir + "/credentials",
	paths.ProjectDataDir + "/streams/",
	paths.ProjectDataDir + "/tmp/",
	paths.ProjectDataDir + "/lock/",
	paths.ProjectDataDir + "/sessions/",
	paths.ProjectDataDir + "/agent-runtime/*",
	"!" + paths.ProjectDataDir + "/agent-runtime/*.example",
	"!" + paths.ProjectDataDir + "/agent-runtime/agent_chat_channel.json",
	"!" + paths.ProjectDataDir + "/agent-runtime/agent_idle_store.json",
	"*.lock",
	paths.ProjectDataDir + "/worktrees/",
	paths.ProjectDataDir + "/**/*.lock",
	paths.ProjectDataDir + "/system-state/cas-indices/",
	paths.ProjectDataDir + "/system-state/snapshots/",
	"!" + paths.ProjectDataDir + "/system-state/README.md",
	paths.ConfigDir + "/zqk-local.yaml",
}

func updateGitignore(gitignorePath string) error {
	// Read existing .gitignore
	content, err := fileutil.ReadFile(gitignorePath)
	if err != nil && !fileutil.IsNotExist(err) {
		return err
	}

	gitignoreContent := string(content)
	lines := strings.Split(gitignoreContent, "\n")
	existing := make(map[string]bool, len(lines)*2)
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			existing[trimmed] = true
			existing[strings.TrimPrefix(trimmed, "/")] = true
		}
	}

	var missing []string
	for _, pat := range defaultRequiredGitignorePatterns {
		cleanPat := strings.TrimPrefix(pat, "/")
		if !existing[pat] && !existing[cleanPat] && !existing["/"+cleanPat] {
			missing = append(missing, pat)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	if gitignoreContent != emptyValue && !endsWithNewline(gitignoreContent) {
		gitignoreContent += "\n"
	}
	gitignoreContent += "\n# ZQK Runtime & Ephemeral Data\n"
	for _, m := range missing {
		gitignoreContent += m + "\n"
	}

	return fileutil.WriteFile(gitignorePath, []byte(gitignoreContent), paths.FilePerm644) //nolint:gosec // Gitignore files - 0600 is acceptable
}

func containsInGitignore(s, substr string) bool {
	// Simple check if substring exists in string
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func endsWithNewline(s string) bool {
	return s != emptyValue && s[len(s)-1] == '\n'
}
