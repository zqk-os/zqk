package git

import (
	"context"

	"github.com/zqk-os/zqk/pkg/gitplumbing"
)

const (
	// DefaultGitRefPrefix is the default root for kernel git plumbing refs.
	DefaultGitRefPrefix = gitplumbing.DefaultGitRefPrefix
	// DefaultGitRemote is the default git remote.
	DefaultGitRemote = gitplumbing.DefaultGitRemote
)

// PlumbingOptions configures the Git plumbing engine.
type PlumbingOptions = gitplumbing.PlumbingOptions

// SnapshotInfo contains metadata about a captured kernel snapshot.
type SnapshotInfo = gitplumbing.SnapshotInfo

// PlumbingEngine manages isolated kernel state distribution via git plumbing.
type PlumbingEngine struct {
	inner *gitplumbing.PlumbingEngine
}

// NewPlumbingEngine creates an initialized PlumbingEngine backed by git plumbing.
func NewPlumbingEngine(opts PlumbingOptions) (*PlumbingEngine, error) {
	inner, err := gitplumbing.NewPlumbingEngine(opts)
	if err != nil {
		return nil, err
	}
	return &PlumbingEngine{inner: inner}, nil
}

// RepoRoot returns the underlying repository root directory.
func (p *PlumbingEngine) RepoRoot() string {
	return p.inner.RepoRoot()
}

// RefPrefix returns the git ref prefix.
func (p *PlumbingEngine) RefPrefix() string {
	return p.inner.RefPrefix()
}

// Snapshot captures the storage directory into an isolated Git tree and commit,
// pointing the given ref (under refPrefix) to it. Does not modify the working tree or index.
func (p *PlumbingEngine) Snapshot(ctx context.Context, refName string, message string) (*SnapshotInfo, error) {
	return p.inner.Snapshot(ctx, refName, message)
}

// Restore unpacks the contents of the given snapshot ref into targetDir
// without touching git working tree or index.
func (p *PlumbingEngine) Restore(ctx context.Context, refName string, targetDir string) error {
	return p.inner.Restore(ctx, refName, targetDir)
}

// Push pushes plumbing refs to remote.
func (p *PlumbingEngine) Push(ctx context.Context, remote string, refSpec string) error {
	return p.inner.Push(ctx, remote, refSpec)
}

// Fetch fetches plumbing refs from remote.
func (p *PlumbingEngine) Fetch(ctx context.Context, remote string, refSpec string) error {
	return p.inner.Fetch(ctx, remote, refSpec)
}

// ListSnapshots lists all snapshots under refPrefix.
func (p *PlumbingEngine) ListSnapshots(ctx context.Context) ([]SnapshotInfo, error) {
	return p.inner.ListSnapshots(ctx)
}
