package git

import (
	"context"
	"strconv"
	"strings"
	"time"

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
