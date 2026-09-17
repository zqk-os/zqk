package scs

import gitpkg "github.com/lanceman/zqk/pkg/git"

// Client defines source-control capabilities used by CLI workflows.
// It is intentionally capability-focused so adapters can map vendor specifics internally.
type Client interface {
	CurrentBranch() (string, error)
	BranchExists(branch string) bool
	CreateBranch(branch string) ([]byte, error)
	CheckoutBranch(branch string) ([]byte, error)
	StatusShort() ([]byte, error)
	AddAll() ([]byte, error)
	DiffCachedStat() ([]byte, error)
	Commit(message string) ([]byte, error)
	Pull(verify bool) ([]byte, error)
	CommitsAheadOneline(branch string) ([]byte, error)
	Push(verify bool) ([]byte, error)
	OriginURL() (string, error)
}

// New returns the default source-control facade implementation for the repo.
// Today this resolves to Git; other source-control adapters can be added later.
func New(repoPath string) Client {
	return gitpkg.NewFacade(repoPath)
}
