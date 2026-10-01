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

// StableBinaryName returns the binary name for stable execution based on brand settings.
func StableBinaryName() string {
	return brand.StableExecutableName()
}

// StableBinaryPath returns the preferred workshop stable CLI path for projectRoot.
func StableBinaryPath(projectRoot string) string {
	return filepath.Join(WorkshopBinDirPath(projectRoot), StableBinaryName())
}

// RepoStableBinaryPath returns bin/<brand>-stable (install.sh dual dest).
func RepoStableBinaryPath(projectRoot string) string {
	binDir := ResolvePathFromCacheOrConstant(projectRoot, PathAliasRepoBin, RepoBinDir)
	return filepath.Join(binDir, StableBinaryName())
}

// StableBinaryCandidates returns workshop then repo stable paths (same inode after promote).
// Long-lived daemons / MCP / host units should prefer these over tip bin/<exe>.
// TRACK: avoid tip/stable split-brain at runtime.
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

// SessionStateRel is the project-relative session file (brand data dir / state / session).
func SessionStateRel() string {
	return filepath.ToSlash(filepath.Join(ProjectDataDir, StateDir, SessionStateFile))
}

// SessionStatePath is the absolute persisted CLI session id file.
func SessionStatePath(projectRoot string) string {
	return filepath.Join(StateDirPath(projectRoot), SessionStateFile)
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

func casYAMLPath(kindDir, hashName string) string {
	if kindDir == "" || hashName == "" {
		return ""
	}
	return filepath.Join(kindDir, hashName+YAMLExtension)
}

// PersonasDirPath returns the process personas CAS directory.
func PersonasDirPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasPersonas, ProcessPersonasDir)
}

// PersonaIndexPath returns the persona listing-index file.
func PersonaIndexPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasPersonaIndex, filepath.Join(ProcessPersonasDir, PersonaIndexFile))
}

// PersonaYAMLPath returns the CAS yaml path for a persona hash from the listing index.
func PersonaYAMLPath(projectRoot, hashName string) string {
	return casYAMLPath(PersonasDirPath(projectRoot), hashName)
}

// AccountsDirPath returns the process accounts CAS directory.
func AccountsDirPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasAccounts, ProcessAccountsDir)
}

// AccountIndexPath returns the account listing-index file.
func AccountIndexPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasAccountIndex, filepath.Join(ProcessAccountsDir, AccountIndexFile))
}

// AccountYAMLPath returns the CAS yaml path for an account hash from the listing index.
func AccountYAMLPath(projectRoot, hashName string) string {
	return casYAMLPath(AccountsDirPath(projectRoot), hashName)
}

// RolesDirPath returns the process roles CAS directory.
func RolesDirPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasRoles, ProcessRolesDir)
}

// RoleIndexPath returns the role listing-index file.
func RoleIndexPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasRoleIndex, filepath.Join(ProcessRolesDir, RoleIndexFile))
}

// RoleYAMLPath returns the CAS yaml path for a role hash from the listing index.
func RoleYAMLPath(projectRoot, hashName string) string {
	return casYAMLPath(RolesDirPath(projectRoot), hashName)
}

// KeystoreDirPath returns the process keystore directory.
func KeystoreDirPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasKeystore, ProcessKeystoreDir)
}

// ObjectSpecsDir returns the object-spec tree (domain buckets or flat).
func ObjectSpecsDir(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasObjectSpecs, ProcessInternalObjectSpecsDir)
}

func credentialsRel() string {
	return filepath.Join(ProjectDataDir, CredentialsFile)
}

// CredentialsPath returns the project-local credentials file.
func CredentialsPath(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(projectRoot, credentialsRel())
}

// ProjectDataDirPath returns the brand data directory under projectRoot.
func ProjectDataDirPath(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(projectRoot, ProjectDataDir)
}

// AuthStrategiesDirPath returns the process auth_strategy CAS directory.
func AuthStrategiesDirPath(projectRoot string) string {
	return ResolvePathFromCacheOrConstant(projectRoot, PathAliasAuthStrategies, ProcessAuthStrategiesDir)
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
