package paths

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/stampmemo"
)

const (
	AgentGitIdentityFile        = "agent_git_identity.env"
	AgentGitIdentityExampleFile = "agent_git_identity.env.example"
	AgentWorkspaceSyncFile      = "agent_workspace_sync.json"
)

// AgentRuntimeRel is a path relative to project root under .zqk/agent-runtime/.
func AgentRuntimeRel(name string) string {
	return filepath.Join(ProjectDataDir, AgentRuntimeDir, name)
}

// AgentRuntimeFilePaths is new path then leftover .zqk/config/ (read fallback).
// TRACK: drop ConfigDir dual-read when: no checkout still writes lite-files under .zqk/config/.
func AgentRuntimeFilePaths(projectRoot, name string) []string {
	return []string{
		filepath.Join(projectRoot, AgentRuntimeRel(name)),
		filepath.Join(projectRoot, ProjectDataDir, ConfigDir, name),
	}
}

// AgentRuntimeFile returns an existing new-or-legacy lite file, or the
// write target under agent-runtime when neither exists.
func AgentRuntimeFile(projectRoot, name string) string {
	cands := AgentRuntimeFilePaths(projectRoot, name)
	if hit := stampmemo.FirstExisting(cands); hit != "" {
		return hit
	}
	return cands[0]
}

// ResolveAgentRuntimePath honors a path alias when that file exists; otherwise
// dual-reads agent-runtime then leftover .zqk/config/; write target is the alias default.
func ResolveAgentRuntimePath(projectRoot, alias, name string) string {
	def := ResolvePathFromCacheOrConstant(projectRoot, alias, AgentRuntimeRel(name))
	if stampmemo.Of(def) != 0 {
		return def
	}
	if hit := stampmemo.FirstExisting(AgentRuntimeFilePaths(projectRoot, name)); hit != "" {
		return hit
	}
	return def
}
