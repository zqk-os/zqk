package gitplumbing

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
	repoRoot   string
	storageDir string
	refPrefix  string
	remoteName string
}

// NewPlumbingEngine creates an initialized PlumbingEngine.
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
		repoRoot:   opts.RepoRoot,
		storageDir: storageDir,
		refPrefix:  refPrefix,
		remoteName: remote,
	}, nil
}

// RepoRoot returns the underlying repository root directory.
func (p *PlumbingEngine) RepoRoot() string {
	return p.repoRoot
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

	info, err := fileutil.Stat(p.storageDir)
	if err != nil {
		return nil, errfmt.Errorf("storage dir not found: %w", err)
	}
	if !info.IsDir() {
		return nil, errfmt.Errorf("storage dir is not a directory: %s", p.storageDir)
	}

	tmpIndex, err := os.CreateTemp("", "zqk-git-index-*")
	if err != nil {
		return nil, errfmt.Errorf("create temp index: %w", err)
	}
	tmpIndexPath := tmpIndex.Name()
	_ = tmpIndex.Close()
	_ = os.Remove(tmpIndexPath)
	defer os.Remove(tmpIndexPath)

	env := append(os.Environ(), "GIT_INDEX_FILE="+tmpIndexPath)

	relStorageDir, err := filepath.Rel(p.repoRoot, p.storageDir)
	if err != nil {
		return nil, errfmt.Errorf("rel storage dir: %w", err)
	}

	addCmd := execwrap.CommandContext(ctx, "git", "add", "--all", "--", relStorageDir)
	addCmd.Dir = p.repoRoot
	addCmd.Env = env
	if out, addErr := addCmd.CombinedOutput(); addErr != nil {
		return nil, errfmt.Errorf("git add to temp index (%s): %w", strings.TrimSpace(string(out)), addErr)
	}

	writeTreeCmd := execwrap.CommandContext(ctx, "git", "write-tree")
	writeTreeCmd.Dir = p.repoRoot
	writeTreeCmd.Env = env
	treeOut, err := writeTreeCmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Errorf("git write-tree (%s): %w", strings.TrimSpace(string(treeOut)), err)
	}
	treeHash := strings.TrimSpace(string(treeOut))

	var parentHash string
	revParseCmd := execwrap.CommandContext(ctx, "git", "rev-parse", "--verify", fullRef)
	revParseCmd.Dir = p.repoRoot
	if pOut, pErr := revParseCmd.CombinedOutput(); pErr == nil {
		parentHash = strings.TrimSpace(string(pOut))
	}

	commitArgs := []string{"commit-tree", treeHash}
	if parentHash != "" {
		commitArgs = append(commitArgs, "-p", parentHash)
	}
	if message == "" {
		message = fmt.Sprintf("zqk kernel snapshot at %s", time.Now().UTC().Format(time.RFC3339))
	}
	commitArgs = append(commitArgs, "-m", message)

	commitTreeCmd := execwrap.CommandContext(ctx, "git", commitArgs...)
	commitTreeCmd.Dir = p.repoRoot
	commitTreeCmd.Env = env
	commitOut, err := commitTreeCmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Errorf("git commit-tree (%s): %w", strings.TrimSpace(string(commitOut)), err)
	}
	commitHash := strings.TrimSpace(string(commitOut))

	updateRefCmd := execwrap.CommandContext(ctx, "git", "update-ref", fullRef, commitHash)
	updateRefCmd.Dir = p.repoRoot
	if uOut, uErr := updateRefCmd.CombinedOutput(); uErr != nil {
		return nil, errfmt.Errorf("git update-ref (%s): %w", strings.TrimSpace(string(uOut)), uErr)
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

	relStorageDir, err := filepath.Rel(p.repoRoot, p.storageDir)
	if err != nil {
		return errfmt.Errorf("rel storage dir: %w", err)
	}

	archiveCmd := execwrap.CommandContext(ctx, "git", "archive", fullRef, relStorageDir)
	archiveCmd.Dir = p.repoRoot
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

		if strings.Contains(header.Name, "..") {
			return errfmt.Errorf("tar entry contains traversal '..': %s", header.Name)
		}

		name := strings.TrimPrefix(header.Name, relStorageDir)
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			continue
		}

		destPath := filepath.Join(targetDir, name)
		cleanTarget := filepath.Clean(targetDir)
		cleanDest := filepath.Clean(destPath)
		if !strings.HasPrefix(cleanDest, cleanTarget+string(filepath.Separator)) && cleanDest != cleanTarget {
			return errfmt.Errorf("tar entry path escapes target directory: %s", header.Name)
		}
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
func (p *PlumbingEngine) Push(ctx context.Context, remote string, refSpec string) error {
	if remote == "" {
		remote = p.remoteName
	}
	if refSpec == "" {
		refSpec = fmt.Sprintf("%s*:%s*", p.refPrefix, p.refPrefix)
	}

	cmd := execwrap.CommandContext(ctx, "git", "push", remote, refSpec)
	cmd.Dir = p.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return errfmt.Errorf("git push %s %s (%s): %w", remote, refSpec, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Fetch fetches plumbing refs from remote.
func (p *PlumbingEngine) Fetch(ctx context.Context, remote string, refSpec string) error {
	if remote == "" {
		remote = p.remoteName
	}
	if refSpec == "" {
		refSpec = fmt.Sprintf("%s*:%s*", p.refPrefix, p.refPrefix)
	}

	cmd := execwrap.CommandContext(ctx, "git", "fetch", remote, refSpec)
	cmd.Dir = p.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return errfmt.Errorf("git fetch %s %s (%s): %w", remote, refSpec, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListSnapshots lists all snapshots under refPrefix.
func (p *PlumbingEngine) ListSnapshots(ctx context.Context) ([]SnapshotInfo, error) {
	cmd := execwrap.CommandContext(ctx, "git", "for-each-ref", "--format=%(refname)|%(objectname)|%(committerdate:iso8601)|%(contents:subject)", p.refPrefix)
	cmd.Dir = p.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errfmt.Errorf("git for-each-ref (%s): %w", strings.TrimSpace(string(out)), err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
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
