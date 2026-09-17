package system

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/objects"
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
		AddExample("Greenfield: Initialize in current directory", "%s system init").
		AddExample("Declarative Seed: Initialize with answer file", "%s system init --answer-file seed.yaml").
		AddExample("Legacy: Initialize existing project", "%s system init --legacy").
		AddExample("Snapshot: Initialize from snapshot (test scenario)", "ZQK_TEST_ROOT=test-scenarios/my-scenario %s system init --from-snapshot snapshot.csnap --wipe").
		AddExample("Snapshot: Merge with existing data", "%s system init --from-snapshot snapshot.csnap --merge").
		AddExample("Legacy + discover: Initialize and report existing objects", "%s system init --legacy --discover").
		AddExample("Greenfield with maintenance jobs", "%s system init --with-maintenance-jobs").
		AddExample("Greenfield with onboarding curriculum", "%s system init --with-onboarding-roadmap").
		ExcludeCommonFlags()

	initCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemInitCommandBuilder(), &cobra.Command{
		Use: "init",
	})
	cli.RequireSession(initCmd, false)
	cli.RequireStorage(initCmd, false)
	cli.RequireSchedulerCheck(initCmd, false)

	cli.BindAsyncProgress(initCmd, func(cmd *cobra.Command, args []string) error {
		return runInit(cmd, projectName, template, force, snapshotPath, answerFilePath, legacy, merge, wipe, discover, withMaintenanceJobs, withOnboardingRoadmap, simple, advanced, importOntology)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(initCmd)

	initCmd.Flags().StringVar(&projectName, "project-name", "", "Project name (defaults to directory name)")
	initCmd.Flags().StringVar(&template, objects.KindTemplate, "standard", "Template to use (standard, minimal)")
	initCmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files")
	initCmd.Flags().StringVar(&snapshotPath, "from-snapshot", "", "Initialize from snapshot file (.csnap or .json)")
	initCmd.Flags().StringVar(&answerFilePath, "answer-file", "", "Declarative kernel seed answer file (.yaml or .json) for non-interactive swarm spawn")
	initCmd.Flags().BoolVar(&legacy, "legacy", false, "Legacy project mode (preserve existing files)")
	initCmd.Flags().BoolVar(&merge, "merge", false, "Merge snapshot data with existing (snapshot mode only)")
	initCmd.Flags().BoolVar(&wipe, "wipe", false, "Wipe existing data before restoring snapshot (requires --force)")
	initCmd.Flags().BoolVar(&discover, "discover", false, "Run the interactive Project Discovery Wizard to capture strategic context, stakeholders, and important dates. If --legacy is provided, it will instead scan and report existing objects.")
	initCmd.Flags().BoolVar(&withMaintenanceJobs, "with-maintenance-jobs", false, "After init, ensure retention and audit-aggregation scheduler jobs exist (same as running 'zqk system ensure-retention-jobs'). Puts the project in optimal maintenance configuration.")
	initCmd.Flags().BoolVar(&withOnboardingRoadmap, "with-onboarding-roadmap", false, "After init, create the onboarding roadmap seed scheduler job. Start the scheduler to run it once and create the priority plan, workstream, and backlog items.")
	initCmd.Flags().BoolVar(&simple, "simple", false, "Initialize with a simple, guided interface (Semantic Bridge Phase 1)")
	initCmd.Flags().BoolVar(&advanced, "advanced", false, "Initialize with an advanced, ontology-aware interface (Semantic Bridge Phase 1)")
	initCmd.Flags().StringVar(&importOntology, "import-ontology", "", "Import an external ontology during initialization (requires --advanced)")

	return initCmd
}

// Helper functions are defined here and shared with init_impl.go

func createProjectDataDir(projectDataDir string, _ bool) error {
	// Create project data directory (typically .zqk)
	if err := fileutil.MkdirAll(projectDataDir, paths.DirPerm755); err != nil {
		return err
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

	for _, dir := range subdirs {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return err
		}
	}

	return nil
}

func createProcessDir(processDir string, _ bool) error {
	// Create .zqk/process directory structure
	subdirs := []string{
		filepath.Join(processDir, "_internal", "object_specs"),
		filepath.Join(processDir, "_internal", "lifecycles"),
		filepath.Join(processDir, "_internal", "documentation"),
		filepath.Join(processDir, "_internal", "traits"),
		filepath.Join(processDir, "backlog_items"),
		filepath.Join(processDir, "policies"),
		filepath.Join(processDir, "requirements"),
		filepath.Join(processDir, objects.KindCriteria),
		filepath.Join(processDir, "test_cases"),
		filepath.Join(processDir, "decisions"),
		filepath.Join(processDir, "goals"),
		filepath.Join(processDir, "milestones"),
		filepath.Join(processDir, "workstreams"),
		filepath.Join(processDir, "priority_plans"),
		filepath.Join(processDir, "questions"),
		filepath.Join(processDir, "doc_entries"),
		filepath.Join(processDir, "missions"),
		filepath.Join(processDir, "visions"),
		filepath.Join(processDir, "strategic_contexts"),
		filepath.Join(processDir, "stakeholder_profiles"),
		filepath.Join(processDir, "important_dates"),
		filepath.Join(processDir, "strategic_plans"),
		filepath.Join(processDir, "architecture"),
		filepath.Join(processDir, "audit", "2025-12"),
		filepath.Join(processDir, "change_journal", "2025-12"),
		filepath.Join(processDir, "planning"),
		filepath.Join(processDir, "scheduler_jobs"),
		filepath.Join(processDir, "certificates"),
	}

	for _, dir := range subdirs {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return err
		}
	}

	return nil
}

// writeProjectConfigFiles writes project config to both canonical (.zqk/config/config.yaml)
// and legacy (.zqk/config.yaml) paths so context and all consumers see it.
func writeProjectConfigFiles(projectDataDir, projectName, template string, force bool) error {
	configDir := filepath.Join(projectDataDir, paths.ConfigDir)
	canonicalPath := filepath.Join(configDir, paths.ProjectConfigFile)
	if _, err := fileutil.Stat(canonicalPath); err == nil && !force {
		return errfmt.Errorf("config file already exists: %s (use --force to overwrite)", canonicalPath)
	}
	content := buildProjectConfigContent(projectName, template)
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		return err
	}
	if err := fileutil.WriteFile(canonicalPath, []byte(content), paths.FilePerm644); err != nil { //nolint:gosec // Config - 0600 acceptable
		return err
	}
	legacyPath := filepath.Join(projectDataDir, paths.ProjectConfigFile)
	_ = fileutil.WriteFile(legacyPath, []byte(content), paths.FilePerm644) //nolint:gosec // Keep in sync for backward compat
	return nil
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

func writeBrandSettings(projectRoot string, force bool) error {
	content := fmt.Sprintf(`# Brand Settings
# Generated by: %[1]s system init

version: "1.0.0"
paths:
  aliases: {}
cli:
  default_context: human
# Knowledge Kernel tip priors (state-commit / pre-commit). Relative paths are from project root.
kernel_state:
  snapshot_backup_dir: ../%[1]s-csnap-backups
  snapshot_backup_keep: 3
`, paths.CLICommandNameDefault)

	pathsToWrite := []string{
		filepath.Join(projectRoot, paths.BrandSettingsFilename),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.BrandSettingsFilename),
	}

	for _, p := range pathsToWrite {
		if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
			return errfmt.Newf("create directory for settings %s", p).Wrap(err)
		}
		if _, err := fileutil.Stat(p); err == nil && !force {
			continue // skip if exists and not forced
		}
		if err := fileutil.WriteFile(p, []byte(content), paths.FilePerm644); err != nil {
			return errfmt.Newf("write settings %s", p).Wrap(err)
		}
	}
	return nil
}

func writeSystemAccount(projectRoot string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	content := fmt.Sprintf(`id: ACC-1785920548450214012-68b850c0
kind: account
schema_version: 2.0.0
namespace_id: zqk:kernel
title: System Account
username: system
status: active
created_by: ACC-1785920548450214012-68b850c0
updated_by: ACC-1785920548450214012-68b850c0
created_at: %s
updated_at: %s
`, now, now)
	accountsDir := filepath.Join(projectRoot, paths.ProcessDir, "accounts")
	if err := fileutil.MkdirAll(accountsDir, paths.DirPerm755); err != nil {
		return err
	}
	// We'll write it using the ID hash like other objects, or just a known filename.
	// `ACC-1785920548450214012-68b850c0` usually hashes to `de28fd5cc03c8a6b1bcdd92cd902d0212e83346707ad8edba9e0fc77784eaf3c.yaml` but for bootstrap we can write to `system.yaml`.
	path := filepath.Join(accountsDir, "system.yaml")
	if _, err := fileutil.Stat(path); err == nil {
		return nil
	}
	return fileutil.WriteFile(path, []byte(content), paths.FilePerm644)
}

func buildProjectConfigContent(projectName, template string) string {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	return fmt.Sprintf(`# ZQK Project Configuration
# Project: %s
# Template: %s
# Generated by: %s system init

project:
  name: %s
  template: %s

storage:
  backend: file
  data_dir: %s

logging:
  profile: human
  level: info
  # error_log_output: combined  # combined | separate (for log capture: single stream vs stdout/stderr to separate files)

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
`, projectName, template, cliCmd, projectName, template, paths.ProcessDir)
}

func createConfigFile(configPath, projectName, template string, force bool) error {
	if _, err := fileutil.Stat(configPath); err == nil && !force {
		return errfmt.Errorf("config file already exists: %s (use --force to overwrite)", configPath)
	}
	return fileutil.WriteFile(configPath, []byte(buildProjectConfigContent(projectName, template)), paths.FilePerm644) //nolint:gosec // Config - 0600 acceptable
}

func updateGitignore(gitignorePath string) error {
	// Read existing .gitignore
	content, err := fileutil.ReadFile(gitignorePath)
	if err != nil && !fileutil.IsNotExist(err) {
		return err
	}

	gitignoreContent := string(content)

	// Check if project data directory is already in .gitignore
	projectDataDirPattern := paths.ProjectDataDir + "/"
	if containsInGitignore(gitignoreContent, projectDataDirPattern) {
		return nil // Already present
	}

	// Append project data directory to .gitignore
	if gitignoreContent != emptyValue && !endsWithNewline(gitignoreContent) {
		gitignoreContent += "\n"
	}
	gitignoreContent += fmt.Sprintf("\n# ZQK state and cache\n%s/cache/\n%s/state/\n", paths.ProjectDataDir, paths.ProjectDataDir)

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
