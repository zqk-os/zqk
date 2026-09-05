package paths

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
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
}
