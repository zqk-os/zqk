package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// GitMaintenanceService provides native Git repository cleanup operations.
type GitMaintenanceService struct {
	dir string
}

// NewGitMaintenanceService creates a new GitMaintenanceService.
func NewGitMaintenanceService(dir string) *GitMaintenanceService {
	return &GitMaintenanceService{
		dir: dir,
	}
}

// PruneStaleBranches deletes stale branches based on a two-tier policy:
// 1. Merged/Closed PR branches inactive for > inactiveDuration (e.g., 24h)
// 2. ANY branch (local or remote) inactive for > 7 days (abandonment threshold)
func (s *GitMaintenanceService) PruneStaleBranches(ctx context.Context, inactiveDuration time.Duration, targetBranches []string) (int, error) {
	logger := logging.GetLoggerFromContext(ctx)
	logging.FluentEvent(logger).Info("Starting comprehensive Git branch pruning sweep").Log()

	// 1. Prune and sync remote state
	if err := s.runGit(ctx, "fetch", "--all", "--prune"); err != nil {
		return 0, fmt.Errorf("failed to fetch from remote: %w", err)
	}

	mergedCutoff := time.Now().Add(-inactiveDuration)
	abandonedCutoff := time.Now().Add(-7 * 24 * time.Hour) // 1 week hard limit
	deletedCount := 0

	// Step A: Collect all branches (local and remote)
	allBranches := s.getAllBranches(ctx)

	// Step B: Identify known merged branches
	mergedBranches := s.getMergedBranches(ctx, targetBranches, logger, mergedCutoff)

	// Step C: Evaluate every branch against the two-tier policy
	for _, branch := range allBranches {
		localBranch := strings.TrimPrefix(branch, "origin/")
		localBranch = strings.TrimPrefix(localBranch, "refs/heads/")
		localBranch = strings.TrimPrefix(localBranch, "refs/remotes/origin/")

		if s.isProtected(localBranch) {
			continue
		}

		commitTimeUnix, err := s.getBranchCommitTime(ctx, branch)
		if err != nil {
			continue
		}
		commitTime := time.Unix(commitTimeUnix, 0)

		shouldDelete := false
		reason := ""

		// Policy 1: Merged and inactive for 24 hours
		if mergedBranches[localBranch] || mergedBranches["origin/"+localBranch] {
			if commitTime.Before(mergedCutoff) {
				shouldDelete = true
				reason = "merged and inactive for >24h"
			}
		}

		// Policy 2: Absolute abandonment (> 1 week of no commits)
		if commitTime.Before(abandonedCutoff) {
			shouldDelete = true
			reason = "abandoned for >1 week"
		}

		// Policy 3: Fully merged/no unique diffs
		if !shouldDelete {
			if !s.hasUniqueDiffs(ctx, branch) {
				shouldDelete = true
				reason = "fully merged (no unique diffs)"
			}
		}

		if shouldDelete {
			logging.FluentEvent(logger).Info(fmt.Sprintf("Deleting branch %s (%s)", localBranch, reason)).Log()

			// Sandbox Enhancement: Physical Worktree Cleanup and GH PR Closure
			s.cleanupWorktree(ctx, localBranch, logger)
			s.closePullRequest(ctx, localBranch, logger)

			// Attempt remote delete
			if strings.HasPrefix(branch, "origin/") || strings.HasPrefix(branch, "refs/remotes/") {
				if err := s.runGit(ctx, "push", "origin", "--delete", localBranch); err != nil {
					logging.FluentEvent(logger).Debug(fmt.Sprintf("Remote delete failed for %s: %v", localBranch, err)).Log()
				}
			} else {
				// The Quarantine Protocol: Soft-delete local branches to refs/archive/
				// This preserves unmerged code in case of errant system object transitions
				archiveRef := "refs/archive/" + localBranch
				_ = s.runGit(ctx, "update-ref", archiveRef, localBranch)
			}

			// Delete the local branch
			_ = s.runGit(ctx, "branch", "-D", localBranch)
			deletedCount++
		}
	}

	logging.FluentEvent(logger).Info(fmt.Sprintf("Git branch pruning complete. Deleted count: %d", deletedCount)).Log()
	return deletedCount, nil
}

func (s *GitMaintenanceService) isProtected(branch string) bool {
	if branch == "main" || branch == "master" {
		return true
	}
	if strings.HasPrefix(branch, "integration/") || strings.HasPrefix(branch, "release/") {
		return true
	}
	return false
}

func (s *GitMaintenanceService) getAllBranches(ctx context.Context) []string {
	var branches []string
	// Get remote branches
	out, err := s.runGitOutput(ctx, "branch", "-r")
	if err == nil {
		for _, b := range strings.Split(out, "\n") {
			b = strings.TrimSpace(b)
			if b != "" && !strings.Contains(b, "->") {
				branches = append(branches, b)
			}
		}
	}
	// Get local branches
	out, err = s.runGitOutput(ctx, "branch", "--format=%(refname:short)")
	if err == nil {
		for _, b := range strings.Split(out, "\n") {
			b = strings.TrimSpace(b)
			if b != "" {
				branches = append(branches, b)
			}
		}
	}
	return branches
}

func (s *GitMaintenanceService) hasUniqueDiffs(ctx context.Context, branch string) bool {
	targets := []string{"main"}
	// Also check integration branches just in case
	out, _ := s.runGitOutput(ctx, "branch", "--list", "integration/*")
	for _, b := range strings.Split(out, "\n") {
		b = strings.TrimSpace(strings.TrimPrefix(b, "* "))
		if b != "" {
			targets = append(targets, b)
		}
	}

	for _, target := range targets {
		cherryOut, err := s.runGitOutput(ctx, "cherry", target, branch)
		if err != nil {
			continue
		}

		hasUnique := false
		lines := strings.Split(strings.TrimSpace(cherryOut), "\n")
		if len(lines) == 1 && lines[0] == "" {
			// No commits strictly on this branch vs target
			return false
		}

		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "+") {
				hasUnique = true
				break
			}
		}

		// If against this target, there are NO unique commits (+)
		if !hasUnique {
			return false
		}
	}
	return true
}

func (s *GitMaintenanceService) getBranchCommitTime(ctx context.Context, branch string) (int64, error) {
	out, err := s.runGitOutput(ctx, "log", "-1", "--format=%at", branch)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(out), 10, 64)
}

//nolint:unparam
func (s *GitMaintenanceService) getMergedBranches(ctx context.Context, targetBranches []string, logger *logging.EventLogger, cutoff time.Time) map[string]bool {
	merged := make(map[string]bool)

	// 1. Strict git history
	for _, target := range targetBranches {
		out, err := s.runGitOutput(ctx, "branch", "-a", "--merged", target)
		if err == nil {
			for _, line := range strings.Split(out, "\n") {
				b := strings.TrimSpace(strings.TrimPrefix(line, "*"))
				if b != "" && !strings.Contains(b, "->") {
					merged[b] = true
				}
			}
		}
	}

	// 2. GitHub PR status
	if _, err := exec.LookPath("gh"); err == nil {
		cmd := execwrap.CommandContext(ctx, "gh", "pr", "list", "--state", "all", "--json", "headRefName,state", "--limit", "1000")
		cmd.Dir = s.dir
		if out, err := cmd.Output(); err == nil {
			var prs []struct {
				HeadRefName string `json:"headRefName"`
				State       string `json:"state"`
			}
			if err := json.Unmarshal(out, &prs); err == nil {
				for _, pr := range prs {
					if pr.State == "MERGED" || pr.State == "CLOSED" {
						merged[pr.HeadRefName] = true
						merged["origin/"+pr.HeadRefName] = true
					}
				}
			}
		}
	}

	return merged
}

func (s *GitMaintenanceService) runGit(ctx context.Context, args ...string) error {
	cmd := execwrap.CommandContext(ctx, "git", args...)
	cmd.Dir = s.dir
	return cmd.Run()
}

func (s *GitMaintenanceService) runGitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := execwrap.CommandContext(ctx, "git", args...)
	cmd.Dir = s.dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (s *GitMaintenanceService) cleanupWorktree(ctx context.Context, branch string, logger *logging.EventLogger) {
	out, err := s.runGitOutput(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return
	}
	var currentWT string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentWT = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch refs/heads/") {
			b := strings.TrimPrefix(line, "branch refs/heads/")
			if b == branch && currentWT != "" {
				logging.FluentEvent(logger).Info(fmt.Sprintf("Removing worktree for branch %s: %s", branch, currentWT)).Log()
				_ = s.runGit(ctx, "worktree", "remove", "-f", currentWT)
				_ = s.runGit(ctx, "worktree", "prune")
			}
		}
	}
}

// CleanupWorktreeAndBranchForID cleans up any git worktrees and branches associated with the given ID (e.g. task/backlog item).
func (s *GitMaintenanceService) CleanupWorktreeAndBranchForID(ctx context.Context, id string) error {
	logger := logging.GetLoggerFromContext(ctx)
	lowerID := strings.ToLower(id)

	// Resolve possible branch names
	var branchNames []string
	if strings.HasPrefix(lowerID, "tsk-") || strings.HasPrefix(lowerID, "bli-") || strings.HasPrefix(lowerID, "hot-") {
		prefix := strings.Split(lowerID, "-")[0]
		branchNames = append(branchNames, fmt.Sprintf("%s/%s", prefix, lowerID))
	} else {
		branchNames = append(branchNames, fmt.Sprintf("task/tsk-%s", lowerID))
	}

	// Also support subagent branch patterns and feature branch patterns containing the ID
	branchNames = append(branchNames, "subagent-"+lowerID)
	branchNames = append(branchNames, "agent/"+lowerID)

	// 1. List worktrees
	out, err := s.runGitOutput(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}

	var currentWT string
	var currentWTBranch string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentWT = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch refs/heads/") {
			currentWTBranch = strings.TrimPrefix(line, "branch refs/heads/")

			// Check if this worktree matches any of our target branch names or contains the lowerID
			matches := false
			for _, b := range branchNames {
				if currentWTBranch == b {
					matches = true
					break
				}
			}
			if !matches && (strings.Contains(currentWTBranch, lowerID) || strings.Contains(currentWT, lowerID)) {
				matches = true
			}

			if matches && currentWT != "" {
				logging.FluentEvent(logger).Info(fmt.Sprintf("CleanupWorktreeAndBranchForID: Removing worktree %s for branch %s", currentWT, currentWTBranch)).Log()
				if err := s.runGit(ctx, "worktree", "remove", "-f", currentWT); err != nil {
					logging.FluentEvent(logger).Warn(fmt.Sprintf("CleanupWorktreeAndBranchForID: failed to remove worktree %s: %v", currentWT, err)).Log()
				}
				// Also run git worktree prune to clean up administrative files immediately
				_ = s.runGit(ctx, "worktree", "prune")

				// Physical Worktree Directory cleanup (force unmount/delete) to prevent file descriptor leaks
				worktreeAdminDir := filepath.Join(s.dir, ".git", "worktrees", filepath.Base(currentWT))
				if err := paths.MustNotDestroyProjectRoot(s.dir, worktreeAdminDir); err != nil {
					logging.FluentEvent(logger).Warn("Skipping worktree admin dir cleanup due to hazard check").Path(worktreeAdminDir).WithError(err).Log()
				} else {
					_ = fileutil.RemoveAll(worktreeAdminDir)
				}

				if err := paths.MustNotDestroyProjectRoot(s.dir, currentWT); err != nil {
					logging.FluentEvent(logger).Warn("Skipping worktree cleanup due to hazard check").Path(currentWT).WithError(err).Log()
				} else {
					_ = fileutil.RemoveAll(currentWT)
				}
			}
		}
	}

	// 2. Delete local branches matching our patterns or containing the ID
	// Let's list local branches to see if any contain our ID
	localBranchesOut, err := s.runGitOutput(ctx, "branch", "--format=%(refname:short)")
	if err == nil {
		for _, b := range strings.Split(localBranchesOut, "\n") {
			b = strings.TrimSpace(b)
			if b == "" {
				continue
			}
			matches := false
			for _, targetName := range branchNames {
				if b == targetName {
					matches = true
					break
				}
			}
			if !matches && strings.Contains(strings.ToLower(b), lowerID) {
				matches = true
			}
			if matches {
				if b != "main" && b != "master" && !strings.HasPrefix(b, "integration/") && !strings.HasPrefix(b, "release/") {
					logging.FluentEvent(logger).Info(fmt.Sprintf("CleanupWorktreeAndBranchForID: Deleting local branch %s", b)).Log()
					_ = s.runGit(ctx, "branch", "-D", b)

					// Force close/unmount file handles by removing the physical ref file
					refFile := filepath.Join(s.dir, ".git", "refs", "heads", b)
					_ = fileutil.Remove(refFile)
				}
			}
		}
	}

	return nil
}

func (s *GitMaintenanceService) closePullRequest(ctx context.Context, branch string, logger *logging.EventLogger) {
	if _, err := exec.LookPath("gh"); err == nil {
		cmd := execwrap.CommandContext(ctx, "gh", "pr", "close", branch)
		cmd.Dir = s.dir
		if err := cmd.Run(); err == nil {
			logging.FluentEvent(logger).Info(fmt.Sprintf("Closed PR for quarantined branch %s", branch)).Log()
		}
	}
}

// PruneQuarantinedRefs lists all refs/archive/*, checks their last commit times,
// and deletes any refs that are older than ageThreshold.
func (s *GitMaintenanceService) PruneQuarantinedRefs(ctx context.Context, ageThreshold time.Duration) (int, error) {
	logger := logging.GetLoggerFromContext(ctx)
	logging.FluentEvent(logger).Info("Starting quarantined refs/archive/ pruning sweep").Log()

	out, err := s.runGitOutput(ctx, "for-each-ref", "--format=%(refname)", "refs/archive/")
	if err != nil {
		return 0, fmt.Errorf("failed to list archived refs: %w", err)
	}

	cutoff := time.Now().Add(-ageThreshold)
	prunedCount := 0

	for _, ref := range strings.Split(out, "\n") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}

		commitTimeUnix, err := s.getBranchCommitTime(ctx, ref)
		if err != nil {
			logging.FluentEvent(logger).Debug(fmt.Sprintf("Failed to get commit time for archived ref %s: %v", ref, err)).Log()
			continue
		}

		commitTime := time.Unix(commitTimeUnix, 0)
		if commitTime.Before(cutoff) {
			logging.FluentEvent(logger).Info(fmt.Sprintf("Pruning quarantined ref %s (inactive since %s)", ref, commitTime.Format(time.RFC3339))).Log()
			if err := s.runGit(ctx, "update-ref", "-d", ref); err != nil {
				logging.FluentEvent(logger).Warn(fmt.Sprintf("Failed to delete archived ref %s: %v", ref, err)).Log()
			} else {
				prunedCount++
			}
		}
	}

	logging.FluentEvent(logger).Info(fmt.Sprintf("Quarantined refs pruning complete. Pruned count: %d", prunedCount)).Log()
	return prunedCount, nil
}
