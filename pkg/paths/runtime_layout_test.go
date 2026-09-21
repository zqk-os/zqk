package paths

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
)

func TestRuntimeLayoutPaths_fallbackWithoutCache(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if got := MCPDirPath(root); got != filepath.Join(root, ProjectDataDir, MCPDir) {
		t.Fatalf("MCPDirPath=%q", got)
	}
	if got := MCPConfigPath(root); got != filepath.Join(root, ProjectDataDir, MCPDir, MCPConfigFile) {
		t.Fatalf("MCPConfigPath=%q", got)
	}
	if got := LogsDirPath(root); got != filepath.Join(root, ProjectDataDir, LogsDir) {
		t.Fatalf("LogsDirPath=%q", got)
	}
	if got := StableBinaryPath(root); got != filepath.Join(root, ProjectDataDir, WorkshopBinDir, brand.ZqkStableName) {
		t.Fatalf("StableBinaryPath=%q", got)
	}
	if got := RepoStableBinaryPath(root); got != filepath.Join(root, RepoBinDir, brand.ZqkStableName) {
		t.Fatalf("RepoStableBinaryPath=%q", got)
	}
	cands := StableBinaryCandidates(root)
	if len(cands) != 2 || cands[0] != StableBinaryPath(root) || cands[1] != RepoStableBinaryPath(root) {
		t.Fatalf("StableBinaryCandidates=%v", cands)
	}
	if got := RepoBinPath(root); got != filepath.Join(root, RepoBinDir, brand.ExecutableName()) {
		t.Fatalf("RepoBinPath=%q", got)
	}
	if got := StateDirPath(root); got != filepath.Join(root, ProjectDataDir, StateDir) {
		t.Fatalf("StateDirPath=%q", got)
	}
	if got := SessionStateRel(); got != filepath.ToSlash(filepath.Join(ProjectDataDir, StateDir, SessionStateFile)) {
		t.Fatalf("SessionStateRel=%q", got)
	}
	if got := SessionStatePath(root); got != filepath.Join(root, ProjectDataDir, StateDir, SessionStateFile) {
		t.Fatalf("SessionStatePath=%q", got)
	}
	if got := ObserverTipsPath(root); got != filepath.Join(root, ProjectDataDir, StateDir, ObserverTipsFile) {
		t.Fatalf("ObserverTipsPath=%q", got)
	}
	if got := PeerSeatsPath(root); got != filepath.Join(root, ProjectDataDir, StateDir, MeshStateSubdir, PeerSeatsFile) {
		t.Fatalf("PeerSeatsPath=%q", got)
	}
	if got := PeerAckAwaitsPath(root); got != filepath.Join(root, ProjectDataDir, StateDir, MeshStateSubdir, PeerAckAwaitsFile) {
		t.Fatalf("PeerAckAwaitsPath=%q", got)
	}
	if got := SwarmInitDirPath(root); got != filepath.Join(root, ProjectDataDir, StateDir, MeshStateSubdir, SwarmInitSubdir) {
		t.Fatalf("SwarmInitDirPath=%q", got)
	}
	if got := ScriptsDirPath(root); got != filepath.Join(root, ScriptsDir) {
		t.Fatalf("ScriptsDirPath=%q", got)
	}
	if got := PersonasDirPath(root); got != filepath.Join(root, ProcessPersonasDir) {
		t.Fatalf("PersonasDirPath=%q", got)
	}
	if got := PersonaIndexPath(root); got != filepath.Join(root, ProcessPersonasDir, PersonaIndexFile) {
		t.Fatalf("PersonaIndexPath=%q", got)
	}
	if got := AccountsDirPath(root); got != filepath.Join(root, ProcessAccountsDir) {
		t.Fatalf("AccountsDirPath=%q", got)
	}
	if got := RolesDirPath(root); got != filepath.Join(root, ProcessRolesDir) {
		t.Fatalf("RolesDirPath=%q", got)
	}
	if got := KeystoreDirPath(root); got != filepath.Join(root, ProcessKeystoreDir) {
		t.Fatalf("KeystoreDirPath=%q", got)
	}
	if got := ObjectSpecsDir(root); got != filepath.Join(root, ProcessInternalObjectSpecsDir) {
		t.Fatalf("ObjectSpecsDir=%q", got)
	}
	if got := CredentialsPath(root); got != filepath.Join(root, ProjectDataDir, CredentialsFile) {
		t.Fatalf("CredentialsPath=%q", got)
	}
	if got := ProjectDataDirPath(root); got != filepath.Join(root, ProjectDataDir) {
		t.Fatalf("ProjectDataDirPath=%q", got)
	}
	if got := AuthStrategiesDirPath(root); got != filepath.Join(root, ProcessAuthStrategiesDir) {
		t.Fatalf("AuthStrategiesDirPath=%q", got)
	}
	if got := WorktreeLocalConfigRel(); got != filepath.ToSlash(filepath.Join(ConfigDir, ZqkLocalConfigFileName)) {
		t.Fatalf("WorktreeLocalConfigRel=%q", got)
	}
	if got := ObjectDraftsRel(); got != filepath.ToSlash(filepath.Join(ProjectDataDir, ObjectDraftsDir)) {
		t.Fatalf("ObjectDraftsRel=%q", got)
	}
	if !IsWorktreeRuntimePorcelain(WorktreeLocalConfigRel()) || !IsWorktreeRuntimePorcelain(ObjectDraftsRel()+"/x.yaml") {
		t.Fatal("IsWorktreeRuntimePorcelain rejected canonical rel paths")
	}
	if IsWorktreeRuntimePorcelain("result.txt") {
		t.Fatal("IsWorktreeRuntimePorcelain matched executor path")
	}
}

func TestRuntimeLayoutPaths_honorPathCache(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ReplacePathCache(root, map[string]string{
		PathAliasMCP:              "custom/mcp",
		PathAliasMeshPeerSeats:    "custom/seats.json",
		PathAliasMeshPeerAckAwait: "custom/awaits.json",
		"scripts":                 "tooling/scripts",
		"logs":                    "custom/logs",
		PathAliasPersonas:         "custom/personas",
		PathAliasPersonaIndex:     filepath.Join("custom/personas", PersonaIndexFile),
	})
	if got := MCPDirPath(root); got != filepath.Join(root, "custom/mcp") {
		t.Fatalf("cached MCPDirPath=%q", got)
	}
	if got := PeerSeatsPath(root); got != filepath.Join(root, "custom/seats.json") {
		t.Fatalf("cached PeerSeatsPath=%q", got)
	}
	if got := PeerAckAwaitsPath(root); got != filepath.Join(root, "custom/awaits.json") {
		t.Fatalf("cached PeerAckAwaitsPath=%q", got)
	}
	if got := ScriptsDirPath(root); got != filepath.Join(root, "tooling/scripts") {
		t.Fatalf("cached ScriptsDirPath=%q", got)
	}
	if got := LogsDirPath(root); got != filepath.Join(root, "custom/logs") {
		t.Fatalf("cached LogsDirPath=%q", got)
	}
	if got := PersonasDirPath(root); got != filepath.Join(root, "custom/personas") {
		t.Fatalf("cached PersonasDirPath=%q", got)
	}
	if got := PersonaIndexPath(root); got != filepath.Join(root, "custom/personas", PersonaIndexFile) {
		t.Fatalf("cached PersonaIndexPath=%q", got)
	}
}
