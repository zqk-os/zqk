package paths

import (
	"path/filepath"
	"testing"
)

// TestDefaultPathAliases_IncludesOperationalRoots ensures high-churn .zqk subtrees are
// addressable via prefix:scheduler, prefix:pre_commit, prefix:drafts for cache-aware resolution.
func TestDefaultPathAliases_IncludesOperationalRoots(t *testing.T) {
	t.Parallel()
	m := DefaultPathAliases()
	if got, want := m["scheduler"], filepath.Join(ProjectDataDir, SchedulerDir); got != want {
		t.Errorf(`aliases["scheduler"] = %q, want %q`, got, want)
	}
	if got, want := m["pre_commit"], filepath.Join(ProjectDataDir, PreCommitDir); got != want {
		t.Errorf(`aliases["pre_commit"] = %q, want %q`, got, want)
	}
	if got, want := m["drafts"], filepath.Join(ProjectDataDir, DraftsDir); got != want {
		t.Errorf(`aliases["drafts"] = %q, want %q`, got, want)
	}
	if got, want := m["object_drafts"], filepath.Join(ProjectDataDir, ObjectDraftsDir); got != want {
		t.Errorf(`aliases["object_drafts"] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasDatacellFeatureFlags], filepath.Join(ProjectDataDir, ConfigDir, FeatureFlagsFile); got != want {
		t.Errorf(`aliases[PathAliasDatacellFeatureFlags] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasDatacellTrayYAML], filepath.Join(ProjectDataDir, TrayYAMLFile); got != want {
		t.Errorf(`aliases[PathAliasDatacellTrayYAML] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasDatacellRuntimeManifest], filepath.Join(ProjectDataDir, ConfigDir, DataCellRuntimeManifestFile); got != want {
		t.Errorf(`aliases[PathAliasDatacellRuntimeManifest] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasDatacellAgentChatChannelConfig], filepath.Join(ProjectDataDir, ConfigDir, AgentChatChannelConfigFile); got != want {
		t.Errorf(`aliases[PathAliasDatacellAgentChatChannelConfig] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasDatacellAgentChatChannelEvents], filepath.Join(ProjectDataDir, LogsDir, IDEHooksLogsSubdir, AgentChatChannelEventsFile); got != want {
		t.Errorf(`aliases[PathAliasDatacellAgentChatChannelEvents] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasDatacellStewardEnqueue], filepath.Join(ProjectDataDir, LogsDir, DataCellLogsSubdir, StewardEnqueueJSONLFile); got != want {
		t.Errorf(`aliases[PathAliasDatacellStewardEnqueue] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasDatacellAgentIdleStore], filepath.Join(ProjectDataDir, ConfigDir, AgentIdleStoreFile); got != want {
		t.Errorf(`aliases[PathAliasDatacellAgentIdleStore] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasMCP], filepath.Join(ProjectDataDir, MCPDir); got != want {
		t.Errorf(`aliases[PathAliasMCP] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasWorkshopBin], filepath.Join(ProjectDataDir, WorkshopBinDir); got != want {
		t.Errorf(`aliases[PathAliasWorkshopBin] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasMeshPeerSeats], filepath.Join(ProjectDataDir, StateDir, MeshStateSubdir, PeerSeatsFile); got != want {
		t.Errorf(`aliases[PathAliasMeshPeerSeats] = %q, want %q`, got, want)
	}
	if got, want := m[PathAliasMeshPeerAckAwait], filepath.Join(ProjectDataDir, StateDir, MeshStateSubdir, PeerAckAwaitsFile); got != want {
		t.Errorf(`aliases[PathAliasMeshPeerAckAwait] = %q, want %q`, got, want)
	}
}
