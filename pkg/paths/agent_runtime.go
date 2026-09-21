package paths

import (
	"path/filepath"
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

// AgentRuntimeFile is the lite-file path under .zqk/agent-runtime/.
func AgentRuntimeFile(projectRoot, name string) string {
	return filepath.Join(projectRoot, AgentRuntimeRel(name))
}

// ResolveAgentRuntimePath honors a path alias; otherwise AgentRuntimeRel.
func ResolveAgentRuntimePath(projectRoot, alias, name string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, alias, AgentRuntimeRel(name))
}
