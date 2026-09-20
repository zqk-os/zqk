package paths

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
)

// MCPDirPath returns the absolute MCP runtime directory under project data.
func MCPDirPath(projectRoot string) string {
	fallback := filepath.Join(ProjectDataDir, MCPDir)
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasMCP, fallback)
}

// MCPConfigPath returns the absolute MCP config.yaml path.
func MCPConfigPath(projectRoot string) string {
	fallback := filepath.Join(ProjectDataDir, MCPDir, MCPConfigFile)
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasMCPConfig, fallback)
}

// LogsDirPath returns the absolute logs directory under project data.
func LogsDirPath(projectRoot string) string {
	fallback := filepath.Join(ProjectDataDir, LogsDir)
	return ResolvePathFromCacheOrConstant(projectRoot, "logs", fallback)
}

// WorkshopBinDirPath returns the absolute workshop bin directory (stable CLI images).
func WorkshopBinDirPath(projectRoot string) string {
	fallback := filepath.Join(ProjectDataDir, WorkshopBinDir)
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasWorkshopBin, fallback)
}

// StableBinaryName returns the binary name for stable execution.
// If brand.ExecutableName() is "zcom", it uses "zcom", preserving ps distinctiveness
// for community edition rather than collapsing to zqk-stable.
func StableBinaryName() string {
	if brand.ExecutableName() == "zcom" {
		return "zcom"
	}
	return brand.ZqkStableName
}

// StableBinaryPath returns the preferred workshop stable CLI path for projectRoot.
func StableBinaryPath(projectRoot string) string {
	return filepath.Join(WorkshopBinDirPath(projectRoot), StableBinaryName())
}

// RepoStableBinaryPath returns bin/<brand>-stable (install-zqk-stable.sh dual dest).
func RepoStableBinaryPath(projectRoot string) string {
	binDir := ResolvePathFromCacheOrConstant(projectRoot, PathAliasRepoBin, RepoBinDir)
	return filepath.Join(binDir, StableBinaryName())
}

// StableBinaryCandidates returns workshop then repo stable paths (same inode after promote).
// Long-lived daemons / MCP / host units should prefer these over tip bin/<exe>.
// TRACK: TDE-1785808957221945000-fcd15e47 — avoid tip/stable split-brain at runtime.
func StableBinaryCandidates(projectRoot string) []string {
	if projectRoot == "" {
		return nil
	}
	return []string{
		StableBinaryPath(projectRoot),
		RepoStableBinaryPath(projectRoot),
	}
}

// RepoBinPath returns the repository-local compiled CLI path (bin/<executable>).
func RepoBinPath(projectRoot string) string {
	binDir := ResolvePathFromCacheOrConstant(projectRoot, PathAliasRepoBin, RepoBinDir)
	return filepath.Join(binDir, brand.ExecutableName())
}

// StateDirPath returns the absolute project state directory (.zqk/state by default).
func StateDirPath(projectRoot string) string {
	fallback := filepath.Join(ProjectDataDir, StateDir)
	return ResolvePathFromCacheOrConstant(projectRoot, "state", fallback)
}

// ObserverTipsPath returns the AST observer coach cache file under state/.
func ObserverTipsPath(projectRoot string) string {
	return filepath.Join(StateDirPath(projectRoot), ObserverTipsFile)
}

// MeshStateDirPath returns the absolute mesh state directory.
func MeshStateDirPath(projectRoot string) string {
	fallback := filepath.Join(ProjectDataDir, StateDir, MeshStateSubdir)
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasMeshState, fallback)
}

// PeerSeatsPath returns the absolute peer_seats.json path.
func PeerSeatsPath(projectRoot string) string {
	fallback := filepath.Join(ProjectDataDir, StateDir, MeshStateSubdir, PeerSeatsFile)
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasMeshPeerSeats, fallback)
}

// PeerAckAwaitsPath returns the absolute peer_ack_awaits.json path.
func PeerAckAwaitsPath(projectRoot string) string {
	fallback := filepath.Join(ProjectDataDir, StateDir, MeshStateSubdir, PeerAckAwaitsFile)
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasMeshPeerAckAwait, fallback)
}

// SwarmInitDirPath returns the swarm-init run-artifact directory (under mesh state).
func SwarmInitDirPath(projectRoot string) string {
	return filepath.Join(MeshStateDirPath(projectRoot), SwarmInitSubdir)
}

// ScriptsDirPath returns the absolute scripts/ directory (path-cache alias "scripts").
func ScriptsDirPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, "scripts", ScriptsDir)
}

// WorktreeLocalConfigRel is the git-relative seated-kernel wedge written by
// BootstrapWorktreeConfig (config/<ZqkLocalConfigFileName>).
func WorktreeLocalConfigRel() string {
	return filepath.ToSlash(filepath.Join(ConfigDir, ZqkLocalConfigFileName))
}

// WorktreeLocalConfigRelYml is the .yml sibling of WorktreeLocalConfigRel when
// the canonical file name ends in .yaml.
func WorktreeLocalConfigRelYml() string {
	rel := WorktreeLocalConfigRel()
	if strings.HasSuffix(rel, ".yaml") {
		return strings.TrimSuffix(rel, ".yaml") + ".yml"
	}
	return rel
}

// ObjectDraftsRel is the git-relative draft-plane directory
// (ProjectDataDir/object_drafts).
func ObjectDraftsRel() string {
	return filepath.ToSlash(filepath.Join(ProjectDataDir, ObjectDraftsDir))
}

// IsWorktreeRuntimePorcelain reports git-relative paths that are worktree
// runtime (local config wedge or isolated draft plane), not executor work.
func IsWorktreeRuntimePorcelain(path string) bool {
	path = filepath.ToSlash(path)
	for _, rel := range []string{WorktreeLocalConfigRel(), WorktreeLocalConfigRelYml()} {
		if rel != "" && (path == rel || strings.HasPrefix(path, rel)) {
			return true
		}
	}
	drafts := ObjectDraftsRel()
	return path == drafts || strings.HasPrefix(path, drafts+"/")
}
