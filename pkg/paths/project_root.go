package paths

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// ResolveProjectRoot returns the project root using a single, explicit precedence so callers
// are always sure which root is in use. Use this everywhere project root is needed.
//
// Terminology: "Workspace root" = topmost directory containing .zqk (where .zqk/current_root lives).
// "Project root" = the directory used for this run (may be workspace root or a nested root).
//
// Precedence:
//  1. ZQK_PROJECT_ROOT — explicit production root (scripting, multi-repo)
//  2. ZQK_TEST_ROOT — explicit test root (test isolation)
//  3. Persisted current root — workspace's .zqk/current_root
//  4. FindNearestProjectRoot(startPath) — CWD-based discovery (walk up; nearest .zqk after filtering)
//
// When either env var is set, it is used and CWD/persisted are skipped for that process.
func ResolveProjectRoot(startPath string) string {
	if v := zqkenv.ProjectRoot().Get(); v != emptyValue {
		abs := v
		if a, err := filepath.Abs(v); err == nil {
			abs = a
		}
		if IsAgentWorktreePath(abs) {
			if settingsRoot := LoadBrandSettingsProjectRoot(abs); settingsRoot != "" && !IsAgentWorktreePath(settingsRoot) {
				return settingsRoot
			}
			if main, err := AgentWorktreeMainRepo(abs); err == nil && IsValidProjectRoot(main) {
				return main
			}
			return ""
		}
		return abs
	}
	if v := zqkenv.TestRoot().Get(); v != emptyValue {
		abs := v
		if a, err := filepath.Abs(v); err == nil {
			abs = a
		}
		if IsAgentWorktreePath(abs) {
			if settingsRoot := LoadBrandSettingsProjectRoot(abs); settingsRoot != "" && !IsAgentWorktreePath(settingsRoot) {
				return settingsRoot
			}
			if main, err := AgentWorktreeMainRepo(abs); err == nil && IsValidProjectRoot(main) {
				return main
			}
			return ""
		}
		return abs
	}
	if workspaceRoot := FindWorkspaceRoot(startPath); workspaceRoot != emptyValue {
		if r := ReadPersistedCurrentRoot(workspaceRoot); r != emptyValue {
			return r
		}
		return workspaceRoot
	}
	root := FindNearestProjectRoot(startPath)
	if root != "" && IsAgentWorktreePath(root) {
		if settingsRoot := LoadBrandSettingsProjectRoot(root); settingsRoot != "" && !IsAgentWorktreePath(settingsRoot) {
			return settingsRoot
		}
		if main, err := AgentWorktreeMainRepo(root); err == nil && IsValidProjectRoot(main) {
			return main
		}
		return ""
	}
	if root == "" {
		if wtRoot := FindWorktreeProjectRoot(startPath); wtRoot != "" {
			return wtRoot
		}
	}
	return root
}

// FindWorkspaceRoot returns the nearest directory (walking up from startPath) that contains .zqk.
// If none is found and startPath is in a linked git worktree, it returns the main repository root.
func FindWorkspaceRoot(startPath string) string {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		dir = startPath
	}
	for {
		gitEntry := filepath.Join(dir, GitWorktreeMetadataEntry)
		if st, err := fileutil.Stat(gitEntry); err == nil {
			if !st.IsDir() {
				if wtRoot := resolveGitWorktreeFile(dir, gitEntry); wtRoot != "" {
					return wtRoot
				}
			}
		}
		if _, err := fileutil.Stat(filepath.Join(dir, ProjectDataDir)); err == nil {
			if !isIgnoredNestedProjectRoot(dir) {
				return dir
			}
		}
		if _, err := fileutil.Stat(gitEntry); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return emptyValue
}

// ReadPersistedCurrentRoot reads workspace's .zqk/current_root and returns the project root path if valid.
func ReadPersistedCurrentRoot(workspaceRoot string) string {
	path := filepath.Join(workspaceRoot, ProjectDataDir, CurrentRootFile)
	data, err := fileutil.ReadFile(path) //nolint:gosec
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if content == emptyValue {
		return ""
	}
	var resolved string
	if filepath.IsAbs(content) {
		resolved = filepath.Clean(content)
	} else {
		resolved = filepath.Clean(filepath.Join(workspaceRoot, content))
	}
	workspaceAbs, _ := filepath.Abs(workspaceRoot)
	resolvedAbs, err := filepath.Abs(resolved)
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(resolvedAbs, workspaceAbs) {
		return ""
	}
	if !IsValidProjectRoot(resolvedAbs) {
		return ""
	}
	return resolvedAbs
}

// IsValidProjectRoot returns true if dir contains .zqk (the project/workspace marker).
func IsValidProjectRoot(dir string) bool {
	_, err := fileutil.Stat(filepath.Join(dir, ProjectDataDir))
	return err == nil
}

func isIgnoredNestedProjectRoot(dir string) bool {
	d := filepath.Clean(dir)
	if filepath.Base(d) != "system" {
		return false
	}
	d = filepath.Dir(d)
	if filepath.Base(d) != "zqk" {
		return false
	}
	d = filepath.Dir(d)
	return filepath.Base(d) == "cmd"
}

// FindNearestProjectRoot discovers a project root by walking up from startPath and collecting every
// directory that contains .zqk. It skips ignored nested markers (cmd/zqk/system/.zqk), then returns
// the nearest remaining candidate (closest to startPath).
func FindNearestProjectRoot(startPath string) string {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		dir = startPath
	}
	var candidates []string
	for {
		gitEntry := filepath.Join(dir, GitWorktreeMetadataEntry)
		if st, err := fileutil.Stat(gitEntry); err == nil {
			if !st.IsDir() {
				if wtRoot := resolveGitWorktreeFile(dir, gitEntry); wtRoot != "" {
					return wtRoot
				}
			}
		}
		if _, err := fileutil.Stat(filepath.Join(dir, ProjectDataDir)); err == nil {
			candidates = append(candidates, dir)
		}
		if _, err := fileutil.Stat(gitEntry); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for _, c := range candidates {
		if !isIgnoredNestedProjectRoot(c) {
			return c
		}
	}
	if wtRoot := FindWorktreeProjectRoot(startPath); wtRoot != "" {
		return wtRoot
	}
	return ""
}

// FindWorktreeProjectRoot attempts to locate the parent repository's project root containing .zqk
// when startPath is within a linked git worktree (where .git is a file referencing the main repo).
func FindWorktreeProjectRoot(startPath string) string {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		dir = startPath
	}
	for {
		gitEntry := filepath.Join(dir, GitWorktreeMetadataEntry)
		st, err := fileutil.Stat(gitEntry)
		if err == nil {
			if !st.IsDir() {
				// .git is a file -> linked git worktree
				if mainRoot := resolveGitWorktreeFile(dir, gitEntry); mainRoot != "" {
					return mainRoot
				}
			}
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func resolveGitWorktreeFile(worktreeDir, gitFilePath string) string {
	b, err := fileutil.ReadFile(gitFilePath)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(b))
	const prefix = "gitdir:"
	if !strings.HasPrefix(strings.ToLower(content), prefix) {
		return ""
	}
	gitDirPath := strings.TrimSpace(content[len(prefix):])
	if !filepath.IsAbs(gitDirPath) {
		gitDirPath = filepath.Clean(filepath.Join(worktreeDir, gitDirPath))
	} else {
		gitDirPath = filepath.Clean(gitDirPath)
	}

	// 1. Check commondir inside gitDirPath
	commondirPath := filepath.Join(gitDirPath, "commondir")
	if cdata, err := fileutil.ReadFile(commondirPath); err == nil {
		commonRel := strings.TrimSpace(string(cdata))
		mainGitDir := filepath.Clean(filepath.Join(gitDirPath, commonRel))
		mainRepo := filepath.Dir(mainGitDir)
		if IsValidProjectRoot(mainRepo) {
			return mainRepo
		}
	}

	// 2. Structural path convention: .git/worktrees/<name>
	// Walking up two levels gets to .git, three levels gets to repo root
	parentGit := filepath.Dir(filepath.Dir(gitDirPath))
	if filepath.Base(parentGit) == GitWorktreeMetadataEntry {
		mainRepo := filepath.Dir(parentGit)
		if IsValidProjectRoot(mainRepo) {
			return mainRepo
		}
	}

	// 3. Fallback: try git rev-parse --git-common-dir
	if main, err := gitCommonWorkingTree(worktreeDir); err == nil && IsValidProjectRoot(main) {
		return main
	}

	return ""
}

