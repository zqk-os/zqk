package datacell

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestRuntimePaths(t *testing.T) {
	t.Parallel()
	root := "/tmp/proj"
	if g, w := FeatureFlagsPath(root), filepath.Join(root, paths.ProjectDataDir, paths.AgentRuntimeDir, paths.FeatureFlagsFile); g != w {
		t.Fatalf("FeatureFlagsPath: got %q want %q", g, w)
	}
	if g, w := CLIHookProfilePath(root), filepath.Join(root, paths.ProjectDataDir, paths.AgentRuntimeDir, paths.CLIHookProfileFile); g != w {
		t.Fatalf("CLIHookProfilePath: got %q want %q", g, w)
	}
	if g, w := TrayYAMLPath(root), filepath.Join(root, paths.ProjectDataDir, paths.TrayYAMLFile); g != w {
		t.Fatalf("TrayYAMLPath: got %q want %q", g, w)
	}
	wantACC := filepath.Join(root, paths.ProjectDataDir, paths.AgentRuntimeDir, paths.AgentChatChannelConfigFile)
	if g := AgentChatChannelConfigPath(root); g != wantACC {
		t.Fatalf("AgentChatChannelConfigPath: got %q want %q", g, wantACC)
	}
	wantACE := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, paths.IDEHooksLogsSubdir, paths.AgentChatChannelEventsFile)
	if g := AgentChatChannelEventsJSONLPath(root); g != wantACE {
		t.Fatalf("AgentChatChannelEventsJSONLPath: got %q want %q", g, wantACE)
	}
	if ProtocolVersion == "" {
		t.Fatal("ProtocolVersion must be non-empty")
	}
	all := AllRuntimePaths(root)
	if all.FeatureFlags != FeatureFlagsPath(root) || all.CLIHookProfile != CLIHookProfilePath(root) ||
		all.TrayYAML != TrayYAMLPath(root) || all.RuntimeManifest != RuntimeManifestPath(root) ||
		all.AgentChatChannelConfig != AgentChatChannelConfigPath(root) ||
		all.AgentChatChannelEvents != AgentChatChannelEventsJSONLPath(root) ||
		all.StewardEnqueue != StewardEnqueueJSONLPath(root) ||
		all.StewardMetrics != StewardMetricsJSONLPath(root) {
		t.Fatalf("AllRuntimePaths mismatch vs individual path helpers: %#v", all)
	}
}

func TestRuntimePaths_RespectPathAliasOverride(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	aliases := maps.Clone(paths.DefaultPathAliases())
	customFF := filepath.Join("alt-layout", "flags.json")
	aliases[paths.PathAliasDatacellFeatureFlags] = customFF
	paths.ReplacePathCache(root, aliases)
	want := filepath.Join(root, customFF)
	if got := FeatureFlagsPath(root); got != want {
		t.Fatalf("FeatureFlagsPath with override: got %q want %q", got, want)
	}
}

func TestStreamCurrentKindDir(t *testing.T) {
	t.Parallel()
	root := "/tmp/zqk"
	kind := "audit_event"
	want := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, paths.StreamCurrentSubdir, kind)
	if got := StreamCurrentKindDir(root, kind); got != want {
		t.Fatalf("StreamCurrentKindDir: got %q want %q", got, want)
	}
	if StreamCurrentKindDir("", kind) != "" || StreamCurrentKindDir(root, "") != "" {
		t.Fatal("StreamCurrentKindDir: want empty for empty root or kind")
	}
}

func TestProcessPrimaryDir(t *testing.T) {
	t.Parallel()
	root := "/tmp/zqk"
	want := filepath.Join(root, paths.ProcessDir)
	if got := ProcessPrimaryDir(root); got != want {
		t.Fatalf("ProcessPrimaryDir: got %q want %q", got, want)
	}
	if ProcessPrimaryDir("") != "" {
		t.Fatal("ProcessPrimaryDir: want empty for empty root")
	}
}

func TestCASEntityPrimaryDir(t *testing.T) {
	t.Parallel()
	root := "/tmp/zqk"
	want := filepath.Join(ProcessPrimaryDir(root), "backlog")
	if got := CASEntityPrimaryDir(root, "backlog"); got != want {
		t.Fatalf("CASEntityPrimaryDir: got %q want %q", got, want)
	}
	if CASEntityPrimaryDir("", "backlog") != "" || CASEntityPrimaryDir(root, "") != "" {
		t.Fatal("CASEntityPrimaryDir: want empty for empty root or entityDir")
	}
}
