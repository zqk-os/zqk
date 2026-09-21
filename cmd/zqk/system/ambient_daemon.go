package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ambientDaemonCmd represents the ambient-daemon command
var ambientDaemonCmd *cobra.Command

// NewAmbientDaemonCmd creates the ambient-daemon command
func NewAmbientDaemonCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAmbientDaemonCommandBuilder(), &cobra.Command{Use: "ambient-daemon"})
	cli.BindAsyncProgress(cmd, runAmbientDaemon)
	ambientDaemonCmd = cmd
	return cmd
}

func runAmbientDaemon(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	configDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.AgentRuntimeDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	ambientFile := filepath.Join(configDir, "ambient_context.json")

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Starting Ambient Context Daemon...\nWriting context to %s\nPress Ctrl+C to stop.\n", ambientFile)))

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	ctx := cmd.Context()

	// Initial harvest
	harvestAmbientContext(ctx, projectRoot, ambientFile)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Harvest ambient context
			harvestAmbientContext(ctx, projectRoot, ambientFile)
		}
	}
}

func harvestAmbientContext(ctx context.Context, projectRoot, ambientFile string) {
	// 1. Get current git branch
	branchCmd := execwrap.CommandContext(ctx, "git", "branch", "--show-current")
	branchCmd.Dir = projectRoot
	branchOut, _ := branchCmd.Output()
	branch := strings.TrimSpace(string(branchOut))

	// 2. Get uncommitted and recently modified files
	statusCmd := execwrap.CommandContext(ctx, "git", "status", "--short")
	statusCmd.Dir = projectRoot
	statusOut, _ := statusCmd.Output()
	modifiedFiles := strings.Split(strings.TrimSpace(string(statusOut)), "\n")
	if len(modifiedFiles) == 1 && modifiedFiles[0] == "" {
		modifiedFiles = []string{}
	}

	// 3. Get recent commits
	logCmd := execwrap.CommandContext(ctx, "git", "log", "-n", "3", "--oneline")
	logCmd.Dir = projectRoot
	logOut, _ := logCmd.Output()
	recentCommits := strings.Split(strings.TrimSpace(string(logOut)), "\n")
	if len(recentCommits) == 1 && recentCommits[0] == "" {
		recentCommits = []string{}
	}

	contextData := map[string]any{
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
		"active_branch":  branch,
		"modified_files": modifiedFiles,
		"recent_commits": recentCommits,
	}

	data, err := json.MarshalIndent(contextData, "", "  ")
	if err != nil {
		return
	}

	// Safely write to file
	tempFile := ambientFile + ".tmp"
	if err := fileutil.WriteSecureFile(tempFile, data); err != nil {
		return
	}
	_ = fileutil.Rename(tempFile, ambientFile)
}
