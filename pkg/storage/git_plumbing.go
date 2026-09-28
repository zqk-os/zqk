package storage

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
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// DefaultGitRefPrefix is the default root for kernel git plumbing refs.
	DefaultGitRefPrefix = "refs/zqk/"
	// DefaultGitRemote is the default git remote.
	DefaultGitRemote = "origin"
)

// GitPlumbingOptions configures the Git plumbing engine.
type GitPlumbingOptions struct {
	RepoRoot   string
	StorageDir string
	RefPrefix  string
	RemoteName string
}

// GitSnapshotInfo contains metadata about a captured kernel snapshot.
type GitSnapshotInfo struct {
	RefName    string    `json:"ref_name" yaml:"ref_name"`
	CommitHash string    `json:"commit_hash" yaml:"commit_hash"`
	TreeHash   string    `json:"tree_hash" yaml:"tree_hash"`
	Timestamp  time.Time `json:"timestamp" yaml:"timestamp"`
	Message    string    `json:"message" yaml:"message"`
}

// GitPlumbingEngine manages isolated kernel state distribution via git plumbing.
type GitPlumbingEngine struct {
	repoRoot   string
	storageDir string
	refPrefix  string
	remoteName string
}

// NewGitPlumbingEngine creates an initialized GitPlumbingEngine.
func NewGitPlumbingEngine(opts GitPlumbingOptions) (*GitPlumbingEngine, error) {
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

	return &GitPlumbingEngine{
		repoRoot:   opts.RepoRoot,
		storageDir: storageDir,
		refPrefix:  refPrefix,
		remoteName: remote,
	}, nil
}

// RepoRoot returns the underlying repository root directory.
func (g *GitPlumbingEngine) RepoRoot() string {
	return g.repoRoot
}

// RefPrefix returns the git ref prefix.
func (g *GitPlumbingEngine) RefPrefix() string {
	return g.refPrefix
}

// Snapshot captures the storage directory into an isolated Git tree and commit,
// pointing the given ref (under refPrefix) to it. Does not modify the working tree or index.
func (g *GitPlumbingEngine) Snapshot(ctx context.Context, refName string, message string) (*GitSnapshotInfo, error) {
	if refName == "" {
		return nil, errfmt.Errorf("ref name is required")
	}
	fullRef := refName
	if !strings.HasPrefix(fullRef, "refs/") {
		fullRef = g.refPrefix + strings.TrimPrefix(refName, "/")
	}

	// Verify storage dir exists
	info, err := fileutil.Stat(g.storageDir)
	if err != nil {
		return nil, errfmt.Errorf("storage dir not found: %w", err)
	}
	if !info.IsDir() {
		return nil, errfmt.Errorf("storage dir is not a directory: %s", g.storageDir)
	}

	// Use temporary git index to build the tree without touching the repo's real index
	tmpIndex, err := os.CreateTemp("", "zqk-git-index-*")
	if err != nil {
		return nil, errfmt.Errorf("create temp index: %w", err)
	}
	tmpIndexPath := tmpIndex.Name()
	_ = tmpIndex.Close()
	// Git expects non-existent file or valid index; a 0-byte file causes 'index file smaller than expected'
	_ = os.Remove(tmpIndexPath)
	defer os.Remove(tmpIndexPath)

	env := append(os.Environ(), "GIT_INDEX_FILE="+tmpIndexPath)

	// Add files under storageDir to temp index.
	// We pass path relative to repoRoot.
	relStorageDir, err := filepath.Rel(g.repoRoot, g.storageDir)
	if err != nil {
		return nil, errfmt.Errorf("rel storage dir: %w", err)
	}

	addCmd := execwrap.CommandContext(ctx, "git", "add", "--all", "--", relStorageDir)
	addCmd.Dir = g.repoRoot
	addCmd.Env = env
	if out, addErr := addCmd.CombinedOutput(); addErr != nil {
		return nil, errfmt.Errorf("git add to temp index (%s): %w", strings.TrimSpace(string(out)), addErr)
	}

	// Write tree
	writeTreeCmd := execwrap.CommandContext(ctx, "git", "write-tree")
	writeTreeCmd.Dir = g.repoRoot
	writeTreeCmd.Env = env
	treeOut, err := writeTreeCmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Errorf("git write-tree (%s): %w", strings.TrimSpace(string(treeOut)), err)
	}
	treeHash := strings.TrimSpace(string(treeOut))

	// Resolve parent commit if ref already exists
	var parentHash string
	revParseCmd := execwrap.CommandContext(ctx, "git", "rev-parse", "--verify", fullRef)
	revParseCmd.Dir = g.repoRoot
	if pOut, pErr := revParseCmd.CombinedOutput(); pErr == nil {
		parentHash = strings.TrimSpace(string(pOut))
	}

	// Commit-tree
	commitArgs := []string{"commit-tree", treeHash}
	if parentHash != "" {
		commitArgs = append(commitArgs, "-p", parentHash)
	}
	if message == "" {
		message = fmt.Sprintf("zqk kernel snapshot at %s", time.Now().UTC().Format(time.RFC3339))
	}
	commitArgs = append(commitArgs, "-m", message)

	commitTreeCmd := execwrap.CommandContext(ctx, "git", commitArgs...)
	commitTreeCmd.Dir = g.repoRoot
	commitTreeCmd.Env = env
	commitOut, err := commitTreeCmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Errorf("git commit-tree (%s): %w", strings.TrimSpace(string(commitOut)), err)
	}
	commitHash := strings.TrimSpace(string(commitOut))

	// Update ref
	updateRefCmd := execwrap.CommandContext(ctx, "git", "update-ref", fullRef, commitHash)
	updateRefCmd.Dir = g.repoRoot
	if uOut, uErr := updateRefCmd.CombinedOutput(); uErr != nil {
		return nil, errfmt.Errorf("git update-ref (%s): %w", strings.TrimSpace(string(uOut)), uErr)
	}

	return &GitSnapshotInfo{
		RefName:    fullRef,
		CommitHash: commitHash,
		TreeHash:   treeHash,
		Timestamp:  time.Now().UTC(),
		Message:    message,
	}, nil
}

// Restore unpacks the contents of the given snapshot ref into targetDir
// without touching git working tree or index.
func (g *GitPlumbingEngine) Restore(ctx context.Context, refName string, targetDir string) error {
	if refName == "" {
		return errfmt.Errorf("ref name is required")
	}
	fullRef := refName
	if !strings.HasPrefix(fullRef, "refs/") {
		fullRef = g.refPrefix + strings.TrimPrefix(refName, "/")
	}

	if err := os.MkdirAll(targetDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf("mkdir target dir: %w", err)
	}

	relStorageDir, err := filepath.Rel(g.repoRoot, g.storageDir)
	if err != nil {
		return errfmt.Errorf("rel storage dir: %w", err)
	}

	// git archive <commit> <path> | tar -x -C <targetDir>
	archiveCmd := execwrap.CommandContext(ctx, "git", "archive", fullRef, relStorageDir)
	archiveCmd.Dir = g.repoRoot
	var tarBuf bytes.Buffer
	archiveCmd.Stdout = &tarBuf
	archiveCmd.Stderr = os.Stderr
	if err := archiveCmd.Run(); err != nil {
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

		// Strip relStorageDir prefix so files unpack cleanly directly into targetDir
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

// Push pushes plumbing refs to remote.
func (g *GitPlumbingEngine) Push(ctx context.Context, remote string, refSpec string) error {
	if remote == "" {
		remote = g.remoteName
	}
	if refSpec == "" {
		refSpec = fmt.Sprintf("%s*:%s*", g.refPrefix, g.refPrefix)
	}

	cmd := execwrap.CommandContext(ctx, "git", "push", remote, refSpec)
	cmd.Dir = g.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return errfmt.Errorf("git push %s %s (%s): %w", remote, refSpec, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Fetch fetches plumbing refs from remote.
func (g *GitPlumbingEngine) Fetch(ctx context.Context, remote string, refSpec string) error {
	if remote == "" {
		remote = g.remoteName
	}
	if refSpec == "" {
		refSpec = fmt.Sprintf("%s*:%s*", g.refPrefix, g.refPrefix)
	}

	cmd := execwrap.CommandContext(ctx, "git", "fetch", remote, refSpec)
	cmd.Dir = g.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return errfmt.Errorf("git fetch %s %s (%s): %w", remote, refSpec, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListSnapshots lists all snapshots under refPrefix.
func (g *GitPlumbingEngine) ListSnapshots(ctx context.Context) ([]GitSnapshotInfo, error) {
	cmd := execwrap.CommandContext(ctx, "git", "for-each-ref", "--format=%(refname)|%(objectname)|%(committerdate:iso8601)|%(contents:subject)", g.refPrefix)
	cmd.Dir = g.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Errorf("git for-each-ref (%s): %w", strings.TrimSpace(string(out)), err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var snapshots []GitSnapshotInfo
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
		snapshots = append(snapshots, GitSnapshotInfo{
			RefName:    parts[0],
			CommitHash: parts[1],
			Timestamp:  ts,
			Message:    parts[3],
		})
	}
	return snapshots, nil
}
