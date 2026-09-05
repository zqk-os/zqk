// Package gitevidence fail-closes backlog complete claims that cite empty or
// unrelated git SHAs (merge theater, docs/process-only CAS renames).
//
// TRACK: REDACTED — reopen false-COMPLETE OPENCORE evidence lane.
package gitevidence

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// ValidateBacklogCommitRefs ensures commit_refs are real non-merge commits that
// change product paths and mention the backlog item id in the commit message.
// Empty refs fail. Merge commits fail (agents were citing PR merges on main).
// Paths only under docs/process/ fail (CAS YAML theater).
func ValidateBacklogCommitRefs(repoRoot, backlogItemID, branchRef string, refs []string) error {
	if os.Getenv(zqkenv.TestBypassGitevidence()) == "1" {
		return nil
	}
	bli := strings.TrimSpace(backlogItemID)
	if bli == "" {
		return fmt.Errorf("backlog item id is required for commit_refs evidence")
	}
	if len(refs) == 0 {
		return fmt.Errorf("commit_refs is empty")
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return fmt.Errorf("git repo root is required for commit_refs evidence")
	}

	var last error
	for _, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			last = fmt.Errorf("commit_refs contains an empty entry")
			continue
		}
		if err := validateOne(root, bli, branchRef, ref); err != nil {
			return err
		}
	}

	if branchRef != "" {
		allCommits, err := gitOutput(root, "log", "--all", "--format=%H", "--grep="+bli)
		if err == nil {
			for _, c := range strings.Split(allCommits, "\n") {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				if _, err := gitOutput(root, "merge-base", "--is-ancestor", c, branchRef); err != nil {
					return fmt.Errorf("duplicate implementation: BLI %s is also implemented in commit %s which is not an ancestor of %s", bli, short(c), branchRef)
				}
			}
		}
	}

	return last
}

func validateOne(repoRoot, bliID, branchRef, ref string) error {
	full, err := gitOutput(repoRoot, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return fmt.Errorf("commit_refs %q is not a resolvable git commit in %s: %w", ref, repoRoot, err)
	}
	full = strings.TrimSpace(full)

	if branchRef != "" {
		if _, err := gitOutput(repoRoot, "merge-base", "--is-ancestor", full, branchRef); err != nil {
			return fmt.Errorf("commit_refs %q is not an ancestor of branch_ref %q (YAML-only promotion attempt)", short(full), branchRef)
		}
	}

	parents, err := gitOutput(repoRoot, "rev-list", "--parents", "-n", "1", full)
	if err != nil {
		return fmt.Errorf("commit_refs %q: cannot read parents: %w", ref, err)
	}
	// "sha p1 p2..." — more than one parent ⇒ merge commit
	fields := strings.Fields(strings.TrimSpace(parents))
	if len(fields) > 2 {
		return fmt.Errorf("commit_refs %q is a merge commit; cite a non-merge commit that implements %s", short(full), bliID)
	}

	msg, err := gitOutput(repoRoot, "log", "-1", "--format=%B", full)
	if err != nil {
		return fmt.Errorf("commit_refs %q: cannot read message: %w", ref, err)
	}
	if !strings.Contains(msg, bliID) {
		return fmt.Errorf("commit_refs %q message does not mention %s (refuse unrelated SHA cites)", short(full), bliID)
	}

	names, err := gitOutput(repoRoot, "show", "--name-only", "--pretty=format:", full)
	if err != nil {
		return fmt.Errorf("commit_refs %q: cannot list files: %w", ref, err)
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
		return fmt.Errorf("commit_refs %q has no product-path changes (docs/process-only CAS is not evidence for %s)", short(full), bliID)
	}
	return nil
}

func isProductEvidencePath(p string) bool {
	p = filepath.ToSlash(p)
	if strings.HasPrefix(p, "docs/process/") {
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
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

// ValidateBranchAncestorOfTrunk ensures the branch_ref is an ancestor of the trunk branch (main).
func ValidateBranchAncestorOfTrunk(repoRoot, branchRef string) error {
	if os.Getenv(zqkenv.TestBypassGitevidence()) == "1" {
		return nil
	}
	branchRef = strings.TrimSpace(branchRef)
	if branchRef == "" {
		return fmt.Errorf("branch_ref is required to check ancestor of trunk")
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return fmt.Errorf("git repo root is required to check ancestor of trunk")
	}

	trunk := "main"

	// Fast check if branchRef resolves
	if _, err := gitOutput(root, "rev-parse", "--verify", branchRef); err != nil {
		// Just fail with a clear message
		return fmt.Errorf("plan cannot complete until its branch is an ancestor of trunk: branch_ref %q is not resolvable", branchRef)
	}

	if _, err := gitOutput(root, "merge-base", "--is-ancestor", branchRef, trunk); err != nil {
		return fmt.Errorf("plan cannot complete until its branch is an ancestor of trunk: branch_ref %q is not an ancestor of %q", branchRef, trunk)
	}

	return nil
}
