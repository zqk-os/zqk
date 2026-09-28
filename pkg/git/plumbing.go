package git

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// DefaultGitRefPrefix is the default root for kernel git plumbing refs.
	DefaultGitRefPrefix = "refs/zqk/"
	// DefaultGitRemote is the default git remote.
	DefaultGitRemote = "origin"
)

// PlumbingOptions configures the Git plumbing engine.
type PlumbingOptions struct {
	RepoRoot   string
	StorageDir string
	RefPrefix  string
	RemoteName string
}

// SnapshotInfo contains metadata about a captured kernel snapshot.
type SnapshotInfo struct {
	RefName    string    `json:"ref_name" yaml:"ref_name"`
	CommitHash string    `json:"commit_hash" yaml:"commit_hash"`
	TreeHash   string    `json:"tree_hash" yaml:"tree_hash"`
	Timestamp  time.Time `json:"timestamp" yaml:"timestamp"`
	Message    string    `json:"message" yaml:"message"`
}

// PlumbingEngine manages isolated kernel state distribution via git plumbing.
type PlumbingEngine struct {
	facade     *Facade
	storageDir string
	refPrefix  string
	remoteName string
}

// NewPlumbingEngine creates an initialized PlumbingEngine backed by the Git facade.
func NewPlumbingEngine(opts PlumbingOptions) (*PlumbingEngine, error) {
	if opts.RepoRoot == "" {
		return nil, errfmt.Errorf("repo root is required")
	}
	refPrefix := opts.RefPrefix
	if refPrefix == "" {
		refPrefix = DefaultGitRefPrefix
	}
	if !strings.HasSuffix(refPrefix, "/") {
		refPrefix += "/"
	}
	remote := opts.RemoteName
	if remote == "" {
		remote = DefaultGitRemote
	}

	storageDir := opts.StorageDir
	if storageDir == "" {
		storageDir = filepath.Join(opts.RepoRoot, paths.ProjectDataDir)
	} else if !filepath.IsAbs(storageDir) {
		storageDir = filepath.Join(opts.RepoRoot, storageDir)
	}

	return &PlumbingEngine{
		facade:     NewFacade(opts.RepoRoot),
		storageDir: storageDir,
		refPrefix:  refPrefix,
		remoteName: remote,
	}, nil
}

// RepoRoot returns the underlying repository root directory.
func (p *PlumbingEngine) RepoRoot() string {
	return p.facade.repoPath
}

// RefPrefix returns the git ref prefix.
func (p *PlumbingEngine) RefPrefix() string {
	return p.refPrefix
}

// Snapshot captures the storage directory into an isolated Git tree and commit,
// pointing the given ref (under refPrefix) to it. Does not modify the working tree or index.
func (p *PlumbingEngine) Snapshot(ctx context.Context, refName string, message string) (*SnapshotInfo, error) {
	if refName == "" {
		return nil, errfmt.Errorf("ref name is required")
	}
	fullRef := refName
	if !strings.HasPrefix(fullRef, "refs/") {
		fullRef = p.refPrefix + strings.TrimPrefix(refName, "/")
	}

	// Verify storage dir exists
	info, err := fileutil.Stat(p.storageDir)
	if err != nil {
		return nil, errfmt.Errorf("storage dir not found: %w", err)
	}
	if !info.IsDir() {
		return nil, errfmt.Errorf("storage dir is not a directory: %s", p.storageDir)
	}

	// Use temporary git index to build the tree without touching the repo's real index
	tmpIndex, err := os.CreateTemp("", "zqk-git-index-*")
	if err != nil {
		return nil, errfmt.Errorf("create temp index: %w", err)
	}
	tmpIndexPath := tmpIndex.Name()
	_ = tmpIndex.Close()
	_ = os.Remove(tmpIndexPath)
	defer os.Remove(tmpIndexPath)

	relStorageDir, err := filepath.Rel(p.facade.repoPath, p.storageDir)
	if err != nil {
		return nil, errfmt.Errorf("rel storage dir: %w", err)
	}

	if err := p.facade.AddToIndex(ctx, tmpIndexPath, relStorageDir); err != nil {
		return nil, err
	}

	treeHash, err := p.facade.WriteTree(ctx, tmpIndexPath)
	if err != nil {
		return nil, err
	}

	parentHash, _ := p.facade.RevParseVerify(ctx, fullRef)

	if message == "" {
		message = fmt.Sprintf("zqk kernel snapshot at %s", time.Now().UTC().Format(time.RFC3339))
	}
	commitHash, err := p.facade.CommitTree(ctx, treeHash, parentHash, message, tmpIndexPath)
	if err != nil {
		return nil, err
	}

	if err := p.facade.UpdateRef(ctx, fullRef, commitHash); err != nil {
		return nil, err
	}

	return &SnapshotInfo{
		RefName:    fullRef,
		CommitHash: commitHash,
		TreeHash:   treeHash,
		Timestamp:  time.Now().UTC(),
		Message:    message,
	}, nil
}

// Restore unpacks the contents of the given snapshot ref into targetDir
// without touching git working tree or index.
func (p *PlumbingEngine) Restore(ctx context.Context, refName string, targetDir string) error {
	if refName == "" {
		return errfmt.Errorf("ref name is required")
	}
	fullRef := refName
	if !strings.HasPrefix(fullRef, "refs/") {
		fullRef = p.refPrefix + strings.TrimPrefix(refName, "/")
	}

	if err := os.MkdirAll(targetDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf("mkdir target dir: %w", err)
	}

	relStorageDir, err := filepath.Rel(p.facade.repoPath, p.storageDir)
	if err != nil {
		return errfmt.Errorf("rel storage dir: %w", err)
	}

	var tarBuf bytes.Buffer
	if err := p.facade.Archive(ctx, fullRef, relStorageDir, &tarBuf); err != nil {
		return errfmt.Errorf("git archive failed: %w", err)
	}

	tarReader := tar.NewReader(&tarBuf)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errfmt.Errorf("tar read: %w", err)
		}

		name := strings.TrimPrefix(header.Name, relStorageDir)
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			continue
		}

		destPath := filepath.Join(targetDir, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, paths.DirPerm755); err != nil {
				return errfmt.Errorf("mkdir %s: %w", destPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), paths.DirPerm755); err != nil {
				return errfmt.Errorf("mkdir parent %s: %w", destPath, err)
			}
			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return errfmt.Errorf("create file %s: %w", destPath, err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				_ = outFile.Close()
				return errfmt.Errorf("write file %s: %w", destPath, err)
			}
			_ = outFile.Close()
		}
	}

	return nil
}

// Push pushes plumbing refs to remote via the git facade.
func (p *PlumbingEngine) Push(ctx context.Context, remote string, refSpec string) error {
	if remote == "" {
		remote = p.remoteName
	}
	if refSpec == "" {
		refSpec = fmt.Sprintf("%s*:%s*", p.refPrefix, p.refPrefix)
	}
	return p.facade.PushRefSpec(ctx, remote, refSpec)
}

// Fetch fetches plumbing refs from remote via the git facade.
func (p *PlumbingEngine) Fetch(ctx context.Context, remote string, refSpec string) error {
	if remote == "" {
		remote = p.remoteName
	}
	if refSpec == "" {
		refSpec = fmt.Sprintf("%s*:%s*", p.refPrefix, p.refPrefix)
	}
	return p.facade.FetchRefSpec(ctx, remote, refSpec)
}

// ListSnapshots lists all snapshots under refPrefix via the git facade.
func (p *PlumbingEngine) ListSnapshots(ctx context.Context) ([]SnapshotInfo, error) {
	lines, err := p.facade.ForEachRef(ctx, "%(refname)|%(objectname)|%(committerdate:iso8601)|%(contents:subject)", p.refPrefix)
	if err != nil {
		return nil, err
	}

	var snapshots []SnapshotInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		ts, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[2])
		snapshots = append(snapshots, SnapshotInfo{
			RefName:    parts[0],
			CommitHash: parts[1],
			Timestamp:  ts,
			Message:    parts[3],
		})
	}
	return snapshots, nil
}
