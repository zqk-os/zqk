package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

const agentWorktreeTempBucket = "zqk-worktrees"

// AgentWorktreeDir returns an isolated git-worktree path for taskID that is
// **not** under projectRoot. Default: $TMPDIR/zqk-worktrees/<repo-key>/<taskID>.
// Override base with env AgentWorktreeRoot (brand-prefixed); in-project
// overrides are ignored so Studio load cannot fork-bomb the kernel tree.
// Kernel: POL-AGENT-WORKTREE-ISOLATION-001. TRACK: BLI-1783831585418122000-c57cd667
// AgentWorktreeContainer is the parent directory that holds per-task worktrees.
func AgentWorktreeContainer(projectRoot string) string {
	absRoot, err := filepath.Abs(strings.TrimSpace(projectRoot))
	if err != nil || absRoot == "" {
		absRoot = projectRoot
	}
	if override := strings.TrimSpace(zqkenv.AgentWorktreeRoot().Get()); override != "" {
		if !pathUnderRoot(absRoot, override) {
			return override
		}
	}
	return filepath.Join(fileutil.TempDir(), agentWorktreeTempBucket, worktreeRepoKey(absRoot))
}

// AgentWorktreeDir returns an isolated git-worktree path for taskID that is
// **not** under projectRoot. Default: $TMPDIR/zqk-worktrees/<repo-key>/<taskID>.
func AgentWorktreeDir(projectRoot, taskID string) string {
	id := strings.TrimSpace(taskID)
	if id == "" {
		id = "unnamed"
	}
	return filepath.Join(AgentWorktreeContainer(projectRoot), id)
}

// AgentWorktreeLookupDirs is the isolated path plus the legacy in-tree location
// so teardown/fail-closed still sees old .zqk/worktrees trees.
func AgentWorktreeLookupDirs(projectRoot, taskID string) []string {
	id := strings.TrimSpace(taskID)
	if id == "" {
		id = "unnamed"
	}
	isolated := AgentWorktreeDir(projectRoot, id)
	legacy := filepath.Join(projectRoot, ProjectDataDir, "worktrees", id)
	if filepath.Clean(legacy) == filepath.Clean(isolated) {
		return []string{isolated}
	}
	return []string{isolated, legacy}
}

func worktreeRepoKey(absRoot string) string {
	sum := sha256.Sum256([]byte(absRoot))
	base := filepath.Base(absRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "repo"
	}
	return base + "-" + hex.EncodeToString(sum[:4])
}

func pathUnderRoot(root, p string) bool {
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, absP)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(fileutil.PathSeparator))
}

// IsAgentWorktreePath reports whether path p is an isolated agent worktree directory.
func IsAgentWorktreePath(p string) bool {
	cleanP := filepath.Clean(strings.TrimSpace(p))
	if cleanP == "" || cleanP == "." {
		return false
	}
	sep := string(filepath.Separator)
	if strings.Contains(cleanP, sep+agentWorktreeTempBucket+sep) || strings.HasSuffix(cleanP, sep+agentWorktreeTempBucket) {
		return true
	}
	if strings.Contains(cleanP, sep+ProjectDataDir+sep+"worktrees"+sep) || strings.HasSuffix(cleanP, sep+ProjectDataDir+sep+"worktrees") {
		return true
	}
	if override := strings.TrimSpace(zqkenv.AgentWorktreeRoot().Get()); override != "" {
		if pathUnderRoot(override, cleanP) {
			return true
		}
	}
	tempBucket := filepath.Join(fileutil.TempDir(), agentWorktreeTempBucket)
	if pathUnderRoot(tempBucket, cleanP) {
		return true
	}
	base := filepath.Base(cleanP)
	if strings.HasPrefix(base, "ATK-") || strings.HasPrefix(base, "atk-") {
		if pathUnderRoot(fileutil.TempDir(), cleanP) || strings.HasPrefix(cleanP, "/tmp") || strings.HasPrefix(cleanP, "/private/tmp") || strings.HasPrefix(cleanP, "/var/tmp") {
			return true
		}
	}
	slash := filepath.ToSlash(cleanP)
	if strings.Contains(slash, "/ATK-") || strings.Contains(slash, "/atk-") {
		if strings.HasPrefix(slash, "/tmp/") || strings.HasPrefix(slash, "/private/tmp/") || strings.HasPrefix(slash, "/var/tmp/") || pathUnderRoot(fileutil.TempDir(), cleanP) {
			return true
		}
	}
	return false
}

// BootstrapWorktreeConfig writes config/zqk-local.yaml in worktreeDir
// binding kernel_state.project_root and paths.project_root to the absolute seated kernel,
// and marks config/zqk-local.yaml as assume-unchanged so it remains clean in porcelain status.
func BootstrapWorktreeConfig(worktreeDir, seatedProjectRoot string) error {
	seatedAbs, err := filepath.Abs(strings.TrimSpace(seatedProjectRoot))
	if err != nil || seatedAbs == "" {
		seatedAbs = seatedProjectRoot
	}
	cfgDir := filepath.Join(worktreeDir, ConfigDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		return err
	}
	configPath := filepath.Join(cfgDir, ZqkLocalConfigFileName)
	content := "# Worktree seated kernel configuration\nkernel_state:\n  project_root: " + seatedAbs + "\npaths:\n  project_root: " + seatedAbs + "\n"
	if err := fileutil.WriteStandardFile(configPath, []byte(content)); err != nil {
		return err
	}
	AssumeUnchangedWorktreeConfig(worktreeDir)
	return nil
}

// AssumeUnchangedWorktreeConfig marks config/zqk-local.yaml as assume-unchanged in git
// within worktreeDir so that local worktree configuration does not appear in git status --porcelain.
func AssumeUnchangedWorktreeConfig(worktreeDir string) {
	relConfig := filepath.ToSlash(filepath.Join(ConfigDir, ZqkLocalConfigFileName))
	cmd := execwrap.Command("git", "update-index", "--assume-unchanged", relConfig)
	cmd.Dir = worktreeDir
	_ = cmd.Run()
}

// BootstrapWorktreeSettings binds worktreeDir to seatedProjectRoot by calling BootstrapWorktreeConfig.
func BootstrapWorktreeSettings(worktreeDir, seatedProjectRoot string) error {
	return BootstrapWorktreeConfig(worktreeDir, seatedProjectRoot)
}
