package datacell

// MembraneReadPaths is the first read-path adapter behind [CellMembrane]: stable path resolution
// for operator and CLI surfaces without reaching past the cell boundary (CRIT-DATACELL-001).
// v1 routes through existing path helpers in this package; [StorageProfile] is honored for future
// profile-specific layouts while keeping one code path today.
type MembraneReadPaths struct {
	Membrane    CellMembrane
	ProjectRoot string
}

func (p MembraneReadPaths) active() bool {
	return p.Membrane != nil && p.ProjectRoot != ""
}

// touchProfile records that the membrane was consulted (future: profile-specific routing).
func (p MembraneReadPaths) touchProfile() {
	if p.Membrane != nil {
		_ = p.Membrane.StorageProfile()
	}
}

// withActivePath runs fn with ProjectRoot when the membrane is active; otherwise returns the zero value of T.
func withActivePath[T any](p MembraneReadPaths, fn func(root string) T) T {
	var zero T
	if !p.active() {
		return zero
	}
	p.touchProfile()
	return fn(p.ProjectRoot)
}

// CLIHookProfilePath returns the persisted CLI hook profile JSON path for this project,
// scoped to the membrane's cell identity.
func (p MembraneReadPaths) CLIHookProfilePath() string {
	return withActivePath(p, CLIHookProfilePath)
}

// FeatureFlagsPath returns the feature flags JSON path (runtime organism slice).
func (p MembraneReadPaths) FeatureFlagsPath() string {
	return withActivePath(p, FeatureFlagsPath)
}

// TrayYAMLPath returns the optional tray manifest YAML path.
func (p MembraneReadPaths) TrayYAMLPath() string {
	return withActivePath(p, TrayYAMLPath)
}

// RuntimeManifestPath returns the optional runtime manifest path.
func (p MembraneReadPaths) RuntimeManifestPath() string {
	return withActivePath(p, RuntimeManifestPath)
}

// AgentChatChannelConfigPath returns the agent chat channel lite-file policy path.
func (p MembraneReadPaths) AgentChatChannelConfigPath() string {
	return withActivePath(p, AgentChatChannelConfigPath)
}

// AgentChatChannelEventsJSONLPath returns the agent chat channel JSONL event log path.
func (p MembraneReadPaths) AgentChatChannelEventsJSONLPath() string {
	return withActivePath(p, AgentChatChannelEventsJSONLPath)
}

// StewardEnqueueJSONLPath returns the steward enqueue JSONL queue path (cell coordinator WAL slice).
func (p MembraneReadPaths) StewardEnqueueJSONLPath() string {
	return withActivePath(p, StewardEnqueueJSONLPath)
}

// StewardMetricsJSONLPath returns the steward metrics JSONL path under .zqk/metrics/.
func (p MembraneReadPaths) StewardMetricsJSONLPath() string {
	return withActivePath(p, StewardMetricsJSONLPath)
}

// AllRuntimePaths returns the v1 runtime organism path bundle through the membrane adapter.
func (p MembraneReadPaths) AllRuntimePaths() RuntimePaths {
	return withActivePath(p, AllRuntimePaths)
}

// StreamCurrentKindDir returns the stream_current directory for kind (ProfileStream).
func (p MembraneReadPaths) StreamCurrentKindDir(kind string) string {
	if kind == "" {
		return ""
	}
	return withActivePath(p, func(root string) string {
		return StreamCurrentKindDir(root, kind)
	})
}

// CASEntityPrimaryDir returns the docs/process directory for a CAS entity segment name (ProfileCASEntity).
func (p MembraneReadPaths) CASEntityPrimaryDir(entityDir string) string {
	if entityDir == "" {
		return ""
	}
	return withActivePath(p, func(root string) string {
		return CASEntityPrimaryDir(root, entityDir)
	})
}

// RuntimeOrganismMembraneReadPaths returns read-path adapters for the v1 runtime organism cell
// (light_file profile: feature flags, CLI hook profile, tray YAML, optional manifest, agent chat channel pilot under .zqk/).
func RuntimeOrganismMembraneReadPaths(projectRoot string) MembraneReadPaths {
	return MembraneReadPaths{
		Membrane:    &MinimalMembrane{Profile: ProfileLightFile, Coord: nil},
		ProjectRoot: projectRoot,
	}
}

// StreamMembraneReadPaths returns read-path adapters for a stream-profile cell (stream_current layout)
// with a no-op coordinator. Use [StreamMembraneReadPathsWithCoordinator] to attach a
// [CellCoordinator] (e.g. pkg/scheduler stream steward enqueue).
func StreamMembraneReadPaths(projectRoot string) MembraneReadPaths {
	return StreamMembraneReadPathsWithCoordinator(projectRoot, nil)
}

// StreamMembraneReadPathsWithCoordinator is like [StreamMembraneReadPaths] but uses coord as the
// stream cell's [CellCoordinator] when non-nil; nil keeps [NoopCellCoordinator] via [MinimalMembrane].
func StreamMembraneReadPathsWithCoordinator(projectRoot string, coord CellCoordinator) MembraneReadPaths {
	return MembraneReadPaths{
		Membrane:    &MinimalMembrane{Profile: ProfileStream, Coord: coord},
		ProjectRoot: projectRoot,
	}
}

// CASEntityMembraneReadPaths returns read-path adapters for a CAS-entity-profile cell (docs/process shards).
func CASEntityMembraneReadPaths(projectRoot string) MembraneReadPaths {
	return MembraneReadPaths{
		Membrane:    &MinimalMembrane{Profile: ProfileCASEntity, Coord: nil},
		ProjectRoot: projectRoot,
	}
}
