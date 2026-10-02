// Package gitevidence fail-closes backlog complete claims that cite empty or
// unrelated git SHAs (merge theater, .zqk/process-only CAS renames).
//
// TRACK: reopen false-COMPLETE OPENCORE evidence lane.
package gitevidence

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// ValidateBacklogCommitHashes ensures commit_hashes are real non-merge commits that
// change product paths and mention the backlog item id in the commit message.
// Empty hashes fail. Merge commits fail (agents were citing PR merges on main).
// Paths only under .zqk/process/ fail (CAS YAML theater).
func ValidateBacklogCommitHashes(repoRoot, backlogItemID, branchRef string, hashes []string) error {
	return ValidateBacklogCommitHashesWithPlan(repoRoot, backlogItemID, "", branchRef, hashes)
}

// ValidateBacklogCommitHashesWithPlan ensures commit_hashes are real non-merge commits that
// change product paths and mention the backlog item id or priority plan id in the commit message.
func ValidateBacklogCommitHashesWithPlan(repoRoot, backlogItemID, planID, branchRef string, hashes []string) error {
	if zqkenv.TestBypassGitevidence().Get() == "1" {
		return nil
	}
	bli := strings.TrimSpace(backlogItemID)
	if bli == "" {
		return fmt.Errorf("backlog item id is required for commit_hashes evidence")
	}
	plan := strings.TrimSpace(planID)
	if len(hashes) == 0 {
		return fmt.Errorf("commit_hashes is empty")
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return fmt.Errorf("git repo root is required for commit_hashes evidence")
	}

	var last error
	for _, raw := range hashes {
		hash := strings.TrimSpace(raw)
		if hash == "" {
			last = fmt.Errorf("commit_hashes contains an empty entry")
			continue
		}
		if err := validateOne(root, bli, plan, branchRef, hash); err != nil {
			return err
		}
	}

	targetBranch := branchRef
	if targetBranch != "" {
		if _, err := gitOutput(root, "rev-parse", "--verify", targetBranch+"^{commit}"); err != nil {
			if _, errMain := gitOutput(root, "rev-parse", "--verify", "main^{commit}"); errMain == nil {
				targetBranch = "main"
			} else {
				targetBranch = "HEAD"
			}
		}
		allCommits, err := gitOutput(root, "log", "--all", "--format=%H", "--grep="+bli)
		if err == nil {
			for _, c := range strings.Split(allCommits, "\n") {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				if _, err := gitOutput(root, "merge-base", "--is-ancestor", c, targetBranch); err != nil {
					return fmt.Errorf("duplicate implementation: BLI %s is also implemented in commit %s which is not an ancestor of %s", bli, short(c), branchRef)
				}
			}
		}
	}

	return last
}

func validateOne(repoRoot, bliID, planID, branchRef, hash string) error {
	full, err := gitOutput(repoRoot, "rev-parse", "--verify", hash+"^{commit}")
	if err != nil {
		return fmt.Errorf("commit_hashes %q is not a resolvable git commit in %s: %w", hash, repoRoot, err)
	}
	full = strings.TrimSpace(full)

	targetRef := branchRef
	if targetRef != "" {
		if _, err := gitOutput(repoRoot, "rev-parse", "--verify", targetRef+"^{commit}"); err != nil {
			if _, errMain := gitOutput(repoRoot, "merge-base", "--is-ancestor", full, "main"); errMain == nil {
				targetRef = "main"
			} else if _, errHead := gitOutput(repoRoot, "merge-base", "--is-ancestor", full, "HEAD"); errHead == nil {
				targetRef = "HEAD"
			}
		}
		if _, err := gitOutput(repoRoot, "merge-base", "--is-ancestor", full, targetRef); err != nil {
			return fmt.Errorf("commit_hashes %q is not an ancestor of branch_name %q (YAML-only promotion attempt)", short(full), branchRef)
		}
	}

	parents, err := gitOutput(repoRoot, "rev-list", "--parents", "-n", "1", full)
	if err != nil {
		return fmt.Errorf("commit_hashes %q: cannot read parents: %w", hash, err)
	}
	// "sha p1 p2..." — more than one parent ⇒ merge commit
	fields := strings.Fields(strings.TrimSpace(parents))
	if len(fields) > 2 {
		return fmt.Errorf("commit_hashes %q is a merge commit; cite a non-merge commit that implements %s", short(full), bliID)
	}

	msg, err := gitOutput(repoRoot, "log", "-1", "--format=%B", full)
	if err != nil {
		return fmt.Errorf("commit_hashes %q: cannot read message: %w", hash, err)
	}
	matched := (bliID != "" && strings.Contains(msg, bliID)) || (planID != "" && strings.Contains(msg, planID))
	if !matched {
		return fmt.Errorf("commit_hashes %q message does not mention %s (refuse unrelated SHA cites)", short(full), bliID)
	}

	names, err := gitOutput(repoRoot, "show", "--name-only", "--pretty=format:", full)
	if err != nil {
		return fmt.Errorf("commit_hashes %q: cannot list files: %w", hash, err)
	}
	var product bool
	for _, line := range strings.Split(names, "\n") {
		p := strings.TrimSpace(line)
		if p == "" {
			continue
		}
		if isProductEvidencePath(p) {
			product = true
			break
		}
	}
	if !product {
		return fmt.Errorf("commit_hashes %q has no product-path changes (process-only CAS is not evidence for %s)", short(full), bliID)
	}
	return nil
}

func isProductEvidencePath(p string) bool {
	p = filepath.ToSlash(p)
	if strings.HasPrefix(p, paths.ProcessDir+"/") || strings.HasPrefix(p, paths.ProcessInternalDir+"/") {
		return false
	}
	// Anything else that landed in the commit counts (code, scripts, strategy docs, README, tests).
	return true
}

func short(full string) string {
	if len(full) > 10 {
		return full[:10]
	}
	return full
}

func gitOutput(repoRoot string, args ...string) (string, error) {
	cmd := execwrap.Command("git", args...)
	cmd.Dir = repoRoot
	stdout, stderr, err := execwrap.RunWithBuffers(cmd)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout, nil
}

// ValidateBranchAncestorOfTrunk ensures the branch_name is an ancestor of the trunk branch (main).
func ValidateBranchAncestorOfTrunk(repoRoot, branchName string) error {
	if zqkenv.TestBypassGitevidence().Get() == "1" {
		return nil
	}
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return fmt.Errorf("branch_name is required to check ancestor of trunk")
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return fmt.Errorf("git repo root is required to check ancestor of trunk")
	}

	trunk := "main"

	// Fast check if branchName resolves
	if _, err := gitOutput(root, "rev-parse", "--verify", branchName); err != nil {
		// Just fail with a clear message
		return fmt.Errorf("plan cannot complete until its branch is an ancestor of trunk: branch_name %q is not resolvable", branchName)
	}

	if _, err := gitOutput(root, "merge-base", "--is-ancestor", branchName, trunk); err != nil {
		return fmt.Errorf("plan cannot complete until its branch is an ancestor of trunk: branch_name %q is not an ancestor of %q", branchName, trunk)
	}

	return nil
}
