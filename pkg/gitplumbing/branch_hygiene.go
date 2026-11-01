package gitplumbing

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
)

const (
	// DefaultTrunkBranch is the primary trunk branch for integration verification.
	DefaultTrunkBranch = "main"

	// IntegrationBranchPrefix is the canonical prefix for time-bounded cycle integration branches.
	IntegrationBranchPrefix = "integration/"
)

// ConvergenceGateResult captures the structural convergence state of an integration branch.
type ConvergenceGateResult struct {
	BranchName  string `json:"branch_name" yaml:"branch_name"`
	TrunkBranch string `json:"trunk_branch" yaml:"trunk_branch"`
	IsMerged    bool   `json:"is_merged" yaml:"is_merged"`
	BehindCount int    `json:"behind_count" yaml:"behind_count"`
	AheadCount  int    `json:"ahead_count" yaml:"ahead_count"`
	CanPrune    bool   `json:"can_prune" yaml:"can_prune"`
}

// BranchHygieneEngine orchestrates automated git branch lifecycle validation,
// post-merge branch pruning, and remote convergence gates.
type BranchHygieneEngine struct {
	repoRoot    string
	remoteName  string
	trunkBranch string
}

// NewBranchHygieneEngine initializes a verified BranchHygieneEngine.
func NewBranchHygieneEngine(repoRoot, remoteName, trunkBranch string) (*BranchHygieneEngine, error) {
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return nil, errfmt.Errorf("repo root is required for branch hygiene engine")
	}

	remote := strings.TrimSpace(remoteName)
	if remote == "" {
		remote = DefaultGitRemote
	}

	trunk := strings.TrimSpace(trunkBranch)
	if trunk == "" {
		trunk = DefaultTrunkBranch
	}

	return &BranchHygieneEngine{
		repoRoot:    root,
		remoteName:  remote,
		trunkBranch: trunk,
	}, nil
}

// ListLocalIntegrationBranches discovers all local branches prefixed with "integration/".
func (e *BranchHygieneEngine) ListLocalIntegrationBranches(ctx context.Context) ([]string, error) {
	cmd := execwrap.CommandContext(ctx, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads/"+IntegrationBranchPrefix)
	cmd.Dir = e.repoRoot

	out, err := cmd.Output()
	if err != nil {
		return nil, errfmt.Errorf("failed to list integration branches: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && strings.HasPrefix(trimmed, IntegrationBranchPrefix) {
			branches = append(branches, trimmed)
		}
	}
	return branches, nil
}

// CurrentBranch returns the currently checked-out branch name.
func (e *BranchHygieneEngine) CurrentBranch(ctx context.Context) (string, error) {
	cmd := execwrap.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = e.repoRoot

	out, err := cmd.Output()
	if err != nil {
		return "", errfmt.Errorf("failed to resolve current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// EvaluateConvergence evaluates whether an integration branch is fully merged into trunk
// and calculates ahead/behind commit divergence metrics.
func (e *BranchHygieneEngine) EvaluateConvergence(ctx context.Context, branchName string) (*ConvergenceGateResult, error) {
	branch := strings.TrimSpace(branchName)
	if branch == "" {
		return nil, errfmt.Errorf("branch name cannot be empty")
	}

	trunkRef := e.trunkBranch
	// Prefer remote tracking trunk if available
	remoteTrunk := fmt.Sprintf("%s/%s", e.remoteName, e.trunkBranch)
	testCmd := execwrap.CommandContext(ctx, "git", "rev-parse", "--verify", remoteTrunk)
	testCmd.Dir = e.repoRoot
	if err := testCmd.Run(); err == nil {
		trunkRef = remoteTrunk
	}

	// 1. Check merge-base ancestor (is branch fully contained in trunk)
	ancestorCmd := execwrap.CommandContext(ctx, "git", "merge-base", "--is-ancestor", branch, trunkRef)
	ancestorCmd.Dir = e.repoRoot
	isMerged := ancestorCmd.Run() == nil

	// 2. Calculate rev-list ahead / behind counts
	countCmd := execwrap.CommandContext(ctx, "git", "rev-list", "--left-right", "--count", fmt.Sprintf("%s...%s", trunkRef, branch))
	countCmd.Dir = e.repoRoot
	countOut, err := countCmd.Output()
	if err != nil {
		return nil, errfmt.Errorf("failed to calculate divergence for %s against %s: %w", branch, trunkRef, err)
	}

	fields := strings.Fields(strings.TrimSpace(string(countOut)))
	behind := 0
	ahead := 0
	if len(fields) >= 2 {
		if b, parseErr := strconv.Atoi(fields[0]); parseErr == nil {
			behind = b
		}
		if a, parseErr := strconv.Atoi(fields[1]); parseErr == nil {
			ahead = a
		}
	}

	current, curErr := e.CurrentBranch(ctx)
	if curErr != nil {
		current = ""
	}
	canPrune := isMerged && branch != current && branch != e.trunkBranch

	return &ConvergenceGateResult{
		BranchName:  branch,
		TrunkBranch: trunkRef,
		IsMerged:    isMerged,
		BehindCount: behind,
		AheadCount:  ahead,
		CanPrune:    canPrune,
	}, nil
}

// PruneMergedBranches deletes local integration branches that have been verified as
// fully merged into the trunk branch. Refuses to prune active checkout or trunk.
func (e *BranchHygieneEngine) PruneMergedBranches(ctx context.Context, dryRun bool) ([]string, error) {
	branches, err := e.ListLocalIntegrationBranches(ctx)
	if err != nil {
		return nil, err
	}

	var pruned []string
	for _, b := range branches {
		result, evalErr := e.EvaluateConvergence(ctx, b)
		if evalErr != nil {
			return pruned, evalErr
		}

		if result.CanPrune {
			if !dryRun {
				delCmd := execwrap.CommandContext(ctx, "git", "branch", "-D", b)
				delCmd.Dir = e.repoRoot
				if delErr := delCmd.Run(); delErr != nil {
					return pruned, errfmt.Errorf("failed to delete branch %s: %w", b, delErr)
				}
			}
			pruned = append(pruned, b)
		}
	}

	return pruned, nil
}
