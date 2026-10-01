package system

import (
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewCacheAuditCmd creates a new cache audit command
func NewCacheAuditCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Audit object ID cache for stale entries and identify files deleted outside CLI",
		"Audit the object ID cache to identify files that were deleted outside the CLI.",
		"",
		"This command:",
		"  - Detects cache entries for files that no longer exist on disk",
		"  - Uses git history to identify who deleted them and when",
		"  - Reports the deletion information for process improvement",
		"  - Optionally cleans stale entries from the cache",
	).
		AddExample("Audit cache and report stale entries", "%s system cache-audit").
		AddExample("Audit and clean stale entries", "%s system cache-audit --clean").
		AddExample("Output as JSON for automation", "%s system cache-audit --format json").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCacheAuditCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use:  "cache-audit",
		RunE: runCacheAudit,
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().Bool("clean", false, "Clean stale entries from cache after audit")
	cmd.Flags().String("format", "table", "Output format (table, json, yaml)")

	return cmd
}

// StaleEntry represents a cache entry for a file that no longer exists
type StaleEntry struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	FilePath    string    `json:"file_path"`
	CachedMTime time.Time `json:"cached_mtime"`
	DeletedBy   string    `json:"deleted_by,omitempty"`
	DeletedAt   string    `json:"deleted_at,omitempty"`
	CommitHash  string    `json:"commit_hash,omitempty"`
	CommitMsg   string    `json:"commit_msg,omitempty"`
}

// CacheAuditResult represents the audit results
type CacheAuditResult struct {
	TotalEntries   int          `json:"total_entries"`
	StaleEntries   int          `json:"stale_entries"`
	StaleEntryList []StaleEntry `json:"stale_entry_list"`
	AuditTime      time.Time    `json:"audit_time"`
	ProjectRoot    string       `json:"project_root"`
}

func runCacheAudit(cmd *cobra.Command, args []string) error {
	return RunCacheAuditViaPipeline(cmd, args)
}

// GitDeletionInfo contains information about a file deletion from git
type GitDeletionInfo struct {
	Author     string
	Date       string
	CommitHash string
	CommitMsg  string
}

// findGitDeletionInfo uses git to find who deleted a file and when
func findGitDeletionInfo(projectRoot, filePath string) *GitDeletionInfo {
	// Get relative path from project root
	relPath, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		return nil
	}

	// Use git log to find when file was deleted
	// git log --diff-filter=D --summary --format="%H|%an|%ad|%s" --date=iso -- <file>
	cmd := execwrap.Command("git", "log", "--diff-filter=D", "--summary",
		"--format=%H|%an|%ad|%s", "--date=iso", "--", relPath)
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	output, err := cmd.Output()
	if err != nil {
		// File might not be in git history, or git might not be available
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return nil
	}

	// Parse first line (most recent deletion)
	parts := strings.SplitN(lines[0], "|", 4)
	if len(parts) != 4 {
		return nil
	}

	return &GitDeletionInfo{
		CommitHash: parts[0],
		Author:     parts[1],
		Date:       parts[2],
		CommitMsg:  parts[3],
	}
}

func outputCacheAuditResults(cmd *cobra.Command, result *CacheAuditResult, format string) error {
	switch format {
	case "json", "yaml":
		return cli.FormatOutput(cmd, result)

	default:
		var buf bytes.Buffer
		buf.WriteString("Cache Audit Results\n")
		buf.WriteString("===================\n\n")
		fmt.Fprintf(&buf, "Total cache entries: %d\n", result.TotalEntries)
		fmt.Fprintf(&buf, "Stale entries: %d\n", result.StaleEntries)
		fmt.Fprintf(&buf, "Audit time: %s\n\n", result.AuditTime.Format(time.RFC3339))

		if len(result.StaleEntryList) > 0 {
			buf.WriteString("Stale Entries (files deleted outside CLI):\n")
			buf.WriteString("-------------------------------------------\n")
			for i := range result.StaleEntryList {
				entry := &result.StaleEntryList[i]
				fmt.Fprintf(&buf, "\n  ID: %s (%s)\n", entry.ID, entry.Kind)
				fmt.Fprintf(&buf, "  File: %s\n", entry.FilePath)
				if entry.DeletedBy != emptyValue {
					fmt.Fprintf(&buf, "  Deleted by: %s\n", entry.DeletedBy)
					fmt.Fprintf(&buf, "  Deleted at: %s\n", entry.DeletedAt)
					if len(entry.CommitHash) >= 8 {
						fmt.Fprintf(&buf, "  Commit: %s\n", entry.CommitHash[:8])
					}
					fmt.Fprintf(&buf, "  Message: %s\n", entry.CommitMsg)
				} else {
					buf.WriteString("  (No git history found - may have been deleted before commit)\n")
				}
			}
		} else {
			buf.WriteString("✓ No stale entries found - cache is in sync with filesystem\n")
		}
		return cli.WriteOutput(cmd, buf.Bytes())
	}
}
