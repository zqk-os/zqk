package git

import (
	"os/exec"
	"strings"
)

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
	cmd := exec.Command(gitCommandName, gitCmdShowRef, gitFlagVerify, gitFlagQuiet, gitRefHeadsPrefix+branch)
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
	cmd := exec.Command(gitCommandName, args...)
	cmd.Dir = f.repoPath
	return cmd.Output()
}

func (f *Facade) combinedOutput(args ...string) ([]byte, error) {
	cmd := exec.Command(gitCommandName, args...)
	cmd.Dir = f.repoPath
	return cmd.CombinedOutput()
}

func (f *Facade) combinedOutputWithVerify(verify bool, args ...string) ([]byte, error) {
	cmd := exec.Command(gitCommandName, args...)
	cmd.Dir = f.repoPath
	if verify {
		cmd.Env = append(cmd.Env, "GIT_VERIFY_SIGNATURES=true")
	}
	return cmd.CombinedOutput()
}
