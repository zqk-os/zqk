package remote

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/git"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Cloner defines an interface for cloning remote repositories.
type Cloner interface {
	Clone(ctx context.Context, cloneURL, destDir string) error
}

type defaultGitCloner struct{}

func (c *defaultGitCloner) Clone(ctx context.Context, cloneURL, destDir string) error {
	return git.CloneShallow(ctx, cloneURL, destDir)
}

// IsRemoteTarget checks if a target identifier represents a remote Git repository.
func IsRemoteTarget(target string) bool {
	if target == "" {
		return false
	}
	if fileutil.Exists(target) {
		return false
	}
	if strings.HasPrefix(target, ".") || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "~") {
		return false
	}
	return strings.HasPrefix(target, "http://") ||
		strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "git@") ||
		strings.HasPrefix(target, "ssh://") ||
		(strings.Count(target, "/") >= 2 && !strings.Contains(target, " "))
}

// NormalizeCloneURL converts a remote identifier (e.g. github.com/org/repo) into a clone URL and slug.
func NormalizeCloneURL(target string) (cloneURL string, repoSlug string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", errfmt.Errorf("empty remote repository target")
	}

	if strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "git@") || strings.HasPrefix(target, "ssh://") {
		cleaned := strings.TrimSuffix(target, ".git")
		parts := strings.Split(cleaned, "/")
		if len(parts) >= 2 {
			repoSlug = strings.Join(parts[len(parts)-2:], "/")
		} else {
			repoSlug = parts[len(parts)-1]
		}
		return target, repoSlug, nil
	}

	// Host-relative like github.com/org/repo
	parts := strings.Split(target, "/")
	if len(parts) < 3 {
		return "", "", errfmt.Errorf("invalid remote repository identifier %q (expected host/org/repo)", target)
	}
	repoSlug = fmt.Sprintf("%s/%s", parts[1], strings.TrimSuffix(parts[2], ".git"))
	cloneURL = fmt.Sprintf("https://%s", target)
	if !strings.HasSuffix(cloneURL, ".git") {
		cloneURL += ".git"
	}
	return cloneURL, repoSlug, nil
}

// ResolveOptions configures remote repository resolution.
type ResolveOptions struct {
	CacheDir string
	Cloner   Cloner
	Force    bool
	Timeout  time.Duration
}

// Resolve pulls or locates a remote swarm package in local cache and returns the path to its swarm.yaml.
func Resolve(ctx context.Context, target string, opts ResolveOptions) (string, error) {
	cloneURL, slug, err := NormalizeCloneURL(target)
	if err != nil {
		return "", err
	}

	cacheRoot := opts.CacheDir
	if cacheRoot == "" {
		home, err := fileutil.UserHomeDir()
		if err != nil {
			return "", errfmt.Newf("cannot resolve user home directory for swarm cache").Wrap(err)
		}
		cacheRoot = filepath.Join(home, paths.ProjectDataDir, "swarms", "cache")
	}

	destDir := filepath.Join(cacheRoot, filepath.FromSlash(slug))
	manifestPath := filepath.Join(destDir, "swarm.yaml")

	if fileutil.Exists(manifestPath) && !opts.Force {
		return manifestPath, nil
	}

	if opts.Force && fileutil.Exists(destDir) {
		_ = fileutil.RemoveAll(destDir)
	}

	if err := fileutil.MkdirAll(filepath.Dir(destDir), paths.DirPerm755); err != nil {
		return "", errfmt.Newf("failed to create cache directory %s", filepath.Dir(destDir)).Wrap(err)
	}

	cloner := opts.Cloner
	if cloner == nil {
		cloner = &defaultGitCloner{}
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	cloneCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := cloner.Clone(cloneCtx, cloneURL, destDir); err != nil {
		return "", errfmt.Newf("failed to clone remote swarm %s", target).Wrap(err)
	}

	if !fileutil.Exists(manifestPath) {
		return "", errfmt.Errorf("remote repository %q does not contain a swarm.yaml manifest in root (%s)", target, manifestPath)
	}

	return manifestPath, nil
}
