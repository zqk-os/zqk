package storage

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

// GitPlumbingOptions configures the Git plumbing engine.
type GitPlumbingOptions = gitplumbing.PlumbingOptions

// GitSnapshotInfo contains metadata about a captured kernel snapshot.
type GitSnapshotInfo = gitplumbing.SnapshotInfo

// GitPlumbingEngine manages isolated kernel state distribution via git plumbing.
type GitPlumbingEngine struct {
	inner *gitplumbing.PlumbingEngine
}

// NewGitPlumbingEngine creates an initialized GitPlumbingEngine.
func NewGitPlumbingEngine(opts GitPlumbingOptions) (*GitPlumbingEngine, error) {
	inner, err := gitplumbing.NewPlumbingEngine(opts)
	if err != nil {
		return nil, err
	}
	return &GitPlumbingEngine{inner: inner}, nil
}

// RepoRoot returns the underlying repository root directory.
func (g *GitPlumbingEngine) RepoRoot() string {
	return g.inner.RepoRoot()
}

// RefPrefix returns the git ref prefix.
func (g *GitPlumbingEngine) RefPrefix() string {
	return g.inner.RefPrefix()
}

// Snapshot captures the storage directory into an isolated Git tree and commit,
// pointing the given ref (under refPrefix) to it. Does not modify the working tree or index.
func (g *GitPlumbingEngine) Snapshot(ctx context.Context, refName string, message string) (*GitSnapshotInfo, error) {
	return g.inner.Snapshot(ctx, refName, message)
}

// Restore unpacks the contents of the given snapshot ref into targetDir
// without touching git working tree or index.
func (g *GitPlumbingEngine) Restore(ctx context.Context, refName string, targetDir string) error {
	return g.inner.Restore(ctx, refName, targetDir)
}

// Push pushes plumbing refs to remote.
func (g *GitPlumbingEngine) Push(ctx context.Context, remote string, refSpec string) error {
	return g.inner.Push(ctx, remote, refSpec)
}

// Fetch fetches plumbing refs from remote.
func (g *GitPlumbingEngine) Fetch(ctx context.Context, remote string, refSpec string) error {
	return g.inner.Fetch(ctx, remote, refSpec)
}

// ListSnapshots lists all snapshots under refPrefix.
func (g *GitPlumbingEngine) ListSnapshots(ctx context.Context) ([]GitSnapshotInfo, error) {
	return g.inner.ListSnapshots(ctx)
}
