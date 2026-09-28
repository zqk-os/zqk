package git

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
)

// AheadProbeTimeout bounds whats-next git ahead count (hot path).
const AheadProbeTimeout = 400 * time.Millisecond

const (
	gitCommandName    = "git"
	gitRefHeadsPrefix = "refs/heads/"
	gitCmdShowRef     = "show-ref"
	gitFlagVerify     = "--verify"
	gitFlagQuiet      = "--quiet"
	gitCmdCheckout    = "checkout"
)

// Facade centralizes git command execution for CLI workflows.
type Facade struct {
	repoPath string
}

func NewFacade(repoPath string) *Facade {
	return &Facade{repoPath: repoPath}
}

// RepoPath returns the root path of the repository managed by this facade.
func (f *Facade) RepoPath() string {
	return f.repoPath
}

func (f *Facade) CurrentBranch() (string, error) {
	out, err := f.output("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (f *Facade) BranchExists(branch string) bool {
	cmd := execwrap.Command(gitCommandName, gitCmdShowRef, gitFlagVerify, gitFlagQuiet, gitRefHeadsPrefix+branch)
	cmd.Dir = f.repoPath
	return cmd.Run() == nil
}

func (f *Facade) CreateBranch(branch string) ([]byte, error) {
	return f.combinedOutput(gitCmdCheckout, "-b", branch)
}

func (f *Facade) CheckoutBranch(branch string) ([]byte, error) {
	return f.combinedOutput(gitCmdCheckout, branch)
}

func (f *Facade) StatusShort() ([]byte, error) {
	return f.output("status", "--short")
}

// LSFiles returns the output of git ls-files with optional arguments.
func (f *Facade) LSFiles(args ...string) ([]byte, error) {
	cmdArgs := append([]string{"ls-files"}, args...)
	return f.output(cmdArgs...)
}

// StatusPorcelain returns the machine-readable git status output.
func (f *Facade) StatusPorcelain(args ...string) ([]byte, error) {
	cmdArgs := append([]string{"status", "--porcelain"}, args...)
	return f.output(cmdArgs...)
}

// StatusIgnoredPorcelain returns the machine-readable git status output including ignored files.
func (f *Facade) StatusIgnoredPorcelain() ([]byte, error) {
	return f.output("status", "--ignored", "--porcelain")
}

func (f *Facade) AddAll() ([]byte, error) {
	return f.combinedOutput("add", "-A")
}

func (f *Facade) DiffCachedStat() ([]byte, error) {
	return f.output("diff", "--cached", "--stat")
}

// Diff executes git diff with the given arguments.
func (f *Facade) Diff(args ...string) ([]byte, error) {
	cmdArgs := append([]string{"diff"}, args...)
	return f.output(cmdArgs...)
}

// Show executes git show with the given arguments.
func (f *Facade) Show(args ...string) ([]byte, error) {
	cmdArgs := append([]string{"show"}, args...)
	return f.output(cmdArgs...)
}

// Grep executes git grep with the given arguments.
func (f *Facade) Grep(args ...string) ([]byte, error) {
	cmdArgs := append([]string{"grep"}, args...)
	return f.output(cmdArgs...)
}

func (f *Facade) Commit(message string) ([]byte, error) {
	return f.combinedOutput("commit", "-m", message)
}

func (f *Facade) Pull(verify bool) ([]byte, error) {
	return f.combinedOutputWithVerify(verify, "pull")
}

func (f *Facade) CommitsAheadOneline(branch string) ([]byte, error) {
	return f.output("log", "origin/"+branch+"..HEAD", "--oneline")
}

// CountAheadUpstream returns how many local commits are not on the tracked
// upstream. 0 if no upstream, not a repo, or the probe times out.
func (f *Facade) CountAheadUpstream() int {
	if f == nil || strings.TrimSpace(f.repoPath) == "" {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), AheadProbeTimeout)
	defer cancel()
	cmd := execwrap.CommandContext(ctx, gitCommandName, "rev-list", "--count", "@{upstream}..HEAD")
	cmd.Dir = f.repoPath
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (f *Facade) Push(verify bool) ([]byte, error) {
	return f.combinedOutputWithVerify(verify, "push")
}

func (f *Facade) OriginURL() (string, error) {
	out, err := f.output("remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (f *Facade) output(args ...string) ([]byte, error) {
	cmd := execwrap.Command(gitCommandName, args...)
	cmd.Dir = f.repoPath
	return cmd.Output()
}

func (f *Facade) combinedOutput(args ...string) ([]byte, error) {
	cmd := execwrap.Command(gitCommandName, args...)
	cmd.Dir = f.repoPath
	return cmd.CombinedOutput()
}

func (f *Facade) combinedOutputWithVerify(verify bool, args ...string) ([]byte, error) {
	cmd := execwrap.Command(gitCommandName, args...)
	cmd.Dir = f.repoPath
	if verify {
		cmd.Env = append(cmd.Env, "GIT_VERIFY_SIGNATURES=true")
	}
	return cmd.CombinedOutput()
}

// AddToIndex adds paths to an isolated index file without modifying the main working tree index.
func (f *Facade) AddToIndex(ctx context.Context, indexFile string, relPath string) error {
	cmd := execwrap.CommandContext(ctx, gitCommandName, "add", "--all", "--", relPath)
	cmd.Dir = f.repoPath
	if indexFile != "" {
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexFile)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return errfmt.Errorf("git add to index (%s): %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// WriteTree writes the current index (or custom GIT_INDEX_FILE) to a git tree object.
func (f *Facade) WriteTree(ctx context.Context, indexFile string) (string, error) {
	cmd := execwrap.CommandContext(ctx, gitCommandName, "write-tree")
	cmd.Dir = f.repoPath
	if indexFile != "" {
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexFile)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errfmt.Errorf("git write-tree (%s): %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CommitTree creates a commit object pointing to a tree with optional parents and message.
func (f *Facade) CommitTree(ctx context.Context, treeHash string, parentHash string, message string, indexFile string) (string, error) {
	commitArgs := []string{"commit-tree", treeHash}
	if parentHash != "" {
		commitArgs = append(commitArgs, "-p", parentHash)
	}
	commitArgs = append(commitArgs, "-m", message)
	cmd := execwrap.CommandContext(ctx, gitCommandName, commitArgs...)
	cmd.Dir = f.repoPath
	if indexFile != "" {
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexFile)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errfmt.Errorf("git commit-tree (%s): %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// UpdateRef updates a git ref to point to a target commit hash.
func (f *Facade) UpdateRef(ctx context.Context, ref string, commitHash string) error {
	cmd := execwrap.CommandContext(ctx, gitCommandName, "update-ref", ref, commitHash)
	cmd.Dir = f.repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return errfmt.Errorf("git update-ref (%s): %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RevParseVerify verifies and resolves a ref to its commit hash. Returns empty string if ref not found.
func (f *Facade) RevParseVerify(ctx context.Context, ref string) (string, error) {
	cmd := execwrap.CommandContext(ctx, gitCommandName, "rev-parse", "--verify", ref)
	cmd.Dir = f.repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errfmt.Errorf("git rev-parse verify (%s): %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Archive pipes a git archive for a given ref and subpath into an io.Writer.
func (f *Facade) Archive(ctx context.Context, ref string, subpath string, out io.Writer) error {
	cmd := execwrap.CommandContext(ctx, gitCommandName, "archive", ref, subpath)
	cmd.Dir = f.repoPath
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// PushRefSpec pushes a specific refspec to a remote.
func (f *Facade) PushRefSpec(ctx context.Context, remote string, refSpec string) error {
	cmd := execwrap.CommandContext(ctx, gitCommandName, "push", remote, refSpec)
	cmd.Dir = f.repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return errfmt.Errorf("git push %s %s (%s): %w", remote, refSpec, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// FetchRefSpec fetches a specific refspec from a remote.
func (f *Facade) FetchRefSpec(ctx context.Context, remote string, refSpec string) error {
	cmd := execwrap.CommandContext(ctx, gitCommandName, "fetch", remote, refSpec)
	cmd.Dir = f.repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return errfmt.Errorf("git fetch %s %s (%s): %w", remote, refSpec, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ForEachRef runs git for-each-ref with a format and pattern.
func (f *Facade) ForEachRef(ctx context.Context, format string, pattern string) ([]string, error) {
	cmd := execwrap.CommandContext(ctx, gitCommandName, "for-each-ref", "--format="+format, pattern)
	cmd.Dir = f.repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Errorf("git for-each-ref (%s): %w", strings.TrimSpace(string(out)), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var result []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result, nil
}
