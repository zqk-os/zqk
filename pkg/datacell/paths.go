package datacell

import (
	"path/filepath"

	"github.com/lanceman/zqk/pkg/paths"
)

// ProtocolVersion is the contract version for the runtime organism layout (paths + future optional manifest).
// Bump when introducing a breaking on-disk layout or a new manifest file that consumers must understand.
const ProtocolVersion = "1"

// FeatureFlagsPath returns the absolute path to the feature flags JSON file.
// Uses the path alias [paths.PathAliasDatacellFeatureFlags] when the cache is built; otherwise
// .zqk/config/feature_flags.json. Override via brand/path settings (see PATH_ALIAS_RESOLUTION.md).
func FeatureFlagsPath(projectRoot string) string {
	fallback := filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.FeatureFlagsFile)
	return paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasDatacellFeatureFlags, fallback)
}

// CLIHookProfilePath returns the absolute path to the CLI hook profile JSON file.
func CLIHookProfilePath(projectRoot string) string {
	fallback := filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.CLIHookProfileFile)
	return paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasDatacellCLIHookProfile, fallback)
}

// TrayYAMLPath returns the absolute path to the optional tray manifest YAML.
func TrayYAMLPath(projectRoot string) string {
	fallback := filepath.Join(paths.ProjectDataDir, paths.TrayYAMLFile)
	return paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasDatacellTrayYAML, fallback)
}

// RuntimePaths holds resolved absolute paths for the v1 datacell runtime organism
// (feature flags, CLI hook profile, tray YAML, optional runtime manifest, agent chat channel pilot,
// steward enqueue JSONL queue, optional steward metrics JSONL under .zqk/metrics/).
type RuntimePaths struct {
	FeatureFlags    string
	CLIHookProfile  string
	TrayYAML        string
	RuntimeManifest string
	// AgentChatChannelConfig is the lite-file policy path (may be missing until the operator creates it).
	AgentChatChannelConfig string
	// AgentChatChannelEvents is the append-only JSONL path for chat-channel events (steward / IDE integration).
	AgentChatChannelEvents string
	// AgentIdleStore is the accumulator state file for agent idleness tracking.
	AgentIdleStore string
	// StewardEnqueue is the append-only JSONL path for coordinator enqueue records (drained by data_cell_envelope_tick).
	StewardEnqueue string
	// StewardMetrics is the append-only JSONL path for steward enqueue/drain metrics (written when metrics recording is enabled).
	StewardMetrics string
}

// AgentChatChannelConfigPath returns the absolute path to agent_chat_channel.json (lite file).
// Uses [paths.PathAliasDatacellAgentChatChannelConfig] when the path cache is built.
func AgentChatChannelConfigPath(projectRoot string) string {
	fallback := filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.AgentChatChannelConfigFile)
	return paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasDatacellAgentChatChannelConfig, fallback)
}

// AgentChatChannelEventsJSONLPath returns the absolute path to agent_chat_channel.jsonl under .zqk/logs/ide-hooks/.
func AgentChatChannelEventsJSONLPath(projectRoot string) string {
	fallback := filepath.Join(paths.ProjectDataDir, paths.LogsDir, paths.IDEHooksLogsSubdir, paths.AgentChatChannelEventsFile)
	return paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasDatacellAgentChatChannelEvents, fallback)
}

// AgentIdleStorePath returns the absolute path to agent_idle_store.json (lite file).
func AgentIdleStorePath(projectRoot string) string {
	fallback := filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.AgentIdleStoreFile)
	return paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasDatacellAgentIdleStore, fallback)
}

// AllRuntimePaths returns all standard paths for projectRoot (path-alias aware).
func AllRuntimePaths(projectRoot string) RuntimePaths {
	return RuntimePaths{
		FeatureFlags:           FeatureFlagsPath(projectRoot),
		CLIHookProfile:         CLIHookProfilePath(projectRoot),
		TrayYAML:               TrayYAMLPath(projectRoot),
		RuntimeManifest:        RuntimeManifestPath(projectRoot),
		AgentChatChannelConfig: AgentChatChannelConfigPath(projectRoot),
		AgentChatChannelEvents: AgentChatChannelEventsJSONLPath(projectRoot),
		AgentIdleStore:         AgentIdleStorePath(projectRoot),
		StewardEnqueue:         StewardEnqueueJSONLPath(projectRoot),
		StewardMetrics:         StewardMetricsJSONLPath(projectRoot),
	}
}

// StreamCurrentKindDir returns the stream_current directory for one kind's cell (v1 layout).
// See STREAM_STORAGE.md and stream_current helpers under pkg/storage.
func StreamCurrentKindDir(projectRoot, kind string) string {
	if projectRoot == "" || kind == "" {
		return ""
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, paths.StreamCurrentSubdir, kind)
}

// ProcessPrimaryDir returns the docs/process tree root under projectRoot (the CAS process prefix).
// Use instead of filepath.Join(projectRoot, paths.ProcessDir).
func ProcessPrimaryDir(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(projectRoot, paths.ProcessDir)
}

// StewardEnqueueJSONLPath returns the append-only JSONL path for data-cell stewardship enqueue
// records (v1). Honors [paths.PathAliasDatacellStewardEnqueue] when the path cache is built;
// fallback matches default layout under .zqk/logs/datacell/. Drained by data_cell_envelope_tick.
func StewardEnqueueJSONLPath(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	fallback := filepath.Join(paths.ProjectDataDir, paths.LogsDir, paths.DataCellLogsSubdir, paths.StewardEnqueueJSONLFile)
	return paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasDatacellStewardEnqueue, fallback)
}

// CASEntityPrimaryDir returns the docs/process storage directory for a CAS-backed cell given the
// entity directory name (e.g. from objects.GetDirectoryFromKind(kind)).
func CASEntityPrimaryDir(projectRoot, entityDir string) string {
	if entityDir == "" {
		return ""
	}
	base := ProcessPrimaryDir(projectRoot)
	if base == "" {
		return ""
	}
	return filepath.Join(base, entityDir)
}
